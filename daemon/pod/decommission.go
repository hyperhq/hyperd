package pod

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"time"

	dockertypes "github.com/docker/engine-api/types"

	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
)

type sandboxOp func(sb *hypervisor.Vm) error
type stateValidator func(state PodState) bool

func (p *XPod) DelayDeleteOn() bool {
	return false
}

func (p *XPod) Stop(graceful int) error {
	err := p.doStopPod(graceful)
	if err != nil {
		return err
	}

	p.Log(DEBUG, "pod stopped, now wait cleanup")
	if cleanup := p.waitStopDone(graceful, "stop container"); !cleanup {
		p.Log(WARNING, "timeout while wait cleanup pod")
		return fmt.Errorf("did not finish clean up in %d seconds", graceful)
	}

	return nil
}

func (p *XPod) ForceQuit() {
	err := p.protectedSandboxOperation(
		func(sb *hypervisor.Vm) error {
			sb.Kill()
			return nil
		},
		time.Second*5,
		"kill pod")
	if err != nil {
		p.Log(ERROR, "force quit failed: %v", err)
	}
}

func (p *XPod) Remove(force bool) error {

	if p.IsRunning() {
		if !force {
			err := fmt.Errorf("pod is running, cannot be removed")
			p.Log(ERROR, err)
			return err
		}
		p.Log(DEBUG, "stop pod before remove")
		p.doStopPod(10)
		if cleanup := p.waitStopDone(60, "Remove Pod"); !cleanup {
			p.Log(WARNING, "timeout while waiting pod stopped")
		}
	}

	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	p.Log(INFO, "removing pod")
	p.statusLock.Lock()
	p.status = S_POD_NONE
	p.statusLock.Unlock()

	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", p.Id()))
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "hosts", p.Id()))

	//TODO get created volumes and remove them
	for id, c := range p.containers {
		p.factory.registry.ReleaseContainer(id, c.SpecName())
		p.factory.engine.ContainerRm(id, &dockertypes.ContainerRmConfig{false, false, false})
	}
	//TODO should remove items in daemondb:	daemon.db.DeletePod(p.Id)
	if p.DelayDeleteOn() {
		p.Log(DEBUG, "should wait periodical clean up")
		p.factory.registry.Release(p.Id())
		return nil
	}

	return nil
}

func (p *XPod) Dissociate() error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	err := dissociateSandbox(p.sandbox, 0)
	p.factory.registry.Release(p.Id())
	for _, c := range p.containers {
		p.factory.registry.ReleaseContainer(c.Id(), c.SpecName())
	}
	if err != nil {
		p.Log(ERROR, "failed to release vm: %v", err)
		return err
	}
	return nil
}

func (p *XPod) Pause() error {
	p.statusLock.Lock()
	if p.status != S_POD_RUNNING {
		p.statusLock.Unlock()
		err := fmt.Errorf("pause: pod state is not valid: %v", p.status)
		p.Log(ERROR, err)
		return err
	}
	p.status = S_POD_PAUSED
	p.statusLock.Unlock()

	err := p.protectedSandboxOperation(
		func(sb *hypervisor.Vm) error {
			return sb.Pause(true)
		},
		time.Second*5,
		"pause pod")

	if err != nil {
		p.Log(WARNING, "pause: roll back status from %v because of: %v", p.status, err)
		p.statusLock.Lock()
		if p.status == S_POD_PAUSED {
			p.status = S_POD_RUNNING
		}
		p.statusLock.Unlock()
	}

	return err
}

func (p *XPod) UnPause() error {
	p.statusLock.Lock()
	if p.status != S_POD_PAUSED {
		p.statusLock.Unlock()
		err := fmt.Errorf("unpause: pod state is not valid: %v", p.status)
		p.Log(ERROR, err)
		return err
	}
	p.status = S_POD_RUNNING
	p.statusLock.Unlock()

	err := p.protectedSandboxOperation(
		func(sb *hypervisor.Vm) error {
			return sb.Pause(false)
		},
		time.Second*5,
		"resume pod")

	if err != nil {
		//TODO, looks not safe we just rollback status here, should we shutdown if unpause failed?
		p.Log(WARNING, "pause: roll back status from %v because of %v", p.status, err)
		p.statusLock.Lock()
		if p.status == S_POD_RUNNING {
			p.status = S_POD_PAUSED
		}
		p.statusLock.Unlock()
	}

	return err
}

func (p *XPod) KillContainer(id string, sig int64) error {
	c, ok := p.containers[id]
	if !ok {
		err := fmt.Errorf("pod does not have a container %s", id)
		p.Log(ERROR, err)
		return err
	}
	c.setKill()
	return p.protectedSandboxOperation(
		func(sb *hypervisor.Vm) error {
			return sb.KillContainer(id, syscall.Signal(sig))
		},
		time.Second*5,
		fmt.Sprintf("Kill container %s with %d", id, sig))
}

func (p *XPod) StopContainer(id string, graceful int) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if !p.IsRunning() {
		err := fmt.Errorf("pod is not running, cannot stop container")
		p.Log(ERROR, err)
		return err
	}

	_, ok := p.containers[id]
	if !ok {
		err := fmt.Errorf("pod does not have a container %s", id)
		p.Log(ERROR, err)
		return err
	}

	err, eMap := p.stopContainers([]string{id}, map[string]bool{id: true}, graceful)
	if err != nil {
		p.Log(ERROR, "fail during stop container %s: %v", id, err)
	}
	if len(eMap) != 0 {
		if e, exist := eMap[id]; exist {
			//override err with the kill error
			err = e
			p.Log(ERROR, "fail during terminating %s: %v", id, err)
		}
	}
	return err
}

func (p *XPod) RemoveContainer(id string) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	c, ok := p.containers[id]
	if !ok {
		err := fmt.Errorf("container %s not found", id)
		p.Log(WARNING, err)
		return nil
	}

	var (
		err  error
		eMap map[string]error
	)

	defer func() {
		if err != nil {
			if err != nil {
				p.statusLock.Lock()
				p.status = S_POD_ERROR
				p.statusLock.Unlock()
			}
		}
	}()

	cvols := c.volumes()

	if p.IsRunning() {
		if c.IsRunning() {
			err, eMap = p.stopContainers([]string{id}, map[string]bool{id: true}, 5)
			if err != nil {
				p.Log(ERROR, "fail during stop container %s: %v", id, err)
				return err
			}
			if len(eMap) != 0 {
				// let's sleep and check the state of container, this
				// will be the last chance we can avoid fall into error state
				// check if there are duplicate kill
				time.Sleep(1000 * time.Millisecond)
				if e, exist := eMap[id]; exist && !c.IsStopped() {
					err = e
					p.Log(ERROR, "fail during terminating %s: %v", id, err)
					return err
				}
			}
		}
		err = c.removeFromSandbox()
		if err != nil {
			c.Log(ERROR, "failed to remove from sandbox")
			return err
		}
	}
	err = c.umountRootVol()
	if err != nil {
		c.Log(ERROR, "failed to umount root volume")
		return err
	}
	err = p.factory.engine.ContainerRm(id, &dockertypes.ContainerRmConfig{})
	if err != nil {
		c.Log(ERROR, "failed to remove container through engine")
		return err
	}
	p.factory.registry.ReleaseContainer(id, c.SpecName())

	removedVols := make([]string, 0, len(cvols))
	for _, cv := range cvols {
		if v, ok := p.volumes[cv.Name]; ok {
			removed, err := v.tryRemoveFromSandbox()
			if err != nil {
				v.Log(ERROR, "failed to unplug vol: %v", err)
				continue
			}
			if !removed {
				v.Log(DEBUG, "volume did not unplug because it is in use")
				continue
			}
			v.Log(DEBUG, "volume unplugged")
			removedVols = append(removedVols, cv.Name)
		}
	}

	//TODO: remove volumes those created during container creating
	//TODO: remove containers in daemondb daemon.db.DeleteP2C(p.Id)

	return nil
}

// protectedSandboxOperation() protect the hypervisor operations, which may
// panic or hang too long time.
func (p *XPod) protectedSandboxOperation(op sandboxOp, timeout time.Duration, comment string) error {
	dangerousOp := func(sb *hypervisor.Vm, errChan chan<- error) {
		defer func() {
			err := recover()
			if err != nil {
				p.Log(ERROR, err)
				if re, ok := err.(error); ok {
					errChan <- re
				} else {
					errChan <- fmt.Errorf("%v", err)
				}
			}
		}()

		if sb != nil {
			errChan <- op(sb)
		} else {
			p.Log(WARNING, "%s: sandbox not existed", comment)
			errChan <- nil
		}
	}

	errChan := make(chan error, 1)

	p.statusLock.Lock()
	go dangerousOp(p.sandbox, errChan)
	p.statusLock.Unlock()

	var timeoutChan <-chan time.Time
	if timeout < 0 {
		timeoutChan = make(chan time.Time, 1)
	} else {
		timeoutChan = time.After(timeout)
	}

	select {
	case err, ok := <-errChan:
		if !ok {
			err := fmt.Errorf("%s: failed to get operation result", comment)
			p.Log(ERROR, err)
			return err
		}
		return err
	case <-timeoutChan:
		err := fmt.Errorf("%s: timeout (%v) during waiting operation result", comment, timeout)
		p.Log(ERROR, err)
		return err
	}
}

func (p *XPod) doStopPod(graceful int) error {
	var err error

	p.statusLock.Lock()
	if p.status != S_POD_RUNNING && p.status != S_POD_STARTING {
		err = fmt.Errorf("only alived pod could be stopped, current %d", p.status)
	} else {
		p.status = S_POD_STOPPING
	}
	p.statusLock.Unlock()
	if err != nil {
		p.Log(ERROR, err)
		return err
	}

	var errChan = make(chan error, 1)

	p.Log(INFO, "going to stop pod")
	go func(sb *hypervisor.Vm) {
		//lock all resource action of the pod, but don't block list/read query
		p.resourceLock.Lock()
		defer p.resourceLock.Unlock()

		//whatever the result of stop container, go on shutdown vm
		p.stopAllContainers(graceful)

		p.Log(INFO, "all container killed (or failed), now shutdown sandbox")
		result := sb.Shutdown()
		if result.IsSuccess() {
			p.Log(INFO, "pod is stopped")
			errChan <- nil
			return
		}

		err := fmt.Errorf("failed to shuting down: %s", result.Message())
		p.Log(ERROR, err)
		errChan <- err
	}(p.sandbox)

	select {
	case err = <-errChan:
		if err != nil {
			p.Log(WARNING, "force quit sandbox due to %v", err)
			p.ForceQuit()
		}
	case <-utils.Timeout(graceful):
		p.Log(WARNING, "force quit sandbox due to timeout")
		p.ForceQuit()
	}

	return nil
}

func (p *XPod) stopAllContainers(graceful int) error {
	//wait all, fire stop signal, and check status,
	if len(p.containers) == 0 {
		p.Log(DEBUG, "no container to be stopped")
		return nil
	}

	var (
		cMap  = make(map[string]bool, len(p.containers))
		cList = make([]string, 0, len(p.containers))
	)

	for cid := range p.containers {
		cMap[cid] = true
		cList = append(cList, cid)
	}

	err, eMap := p.stopContainers(cList, cMap, graceful)
	if err != nil {
		p.Log(ERROR, "exception during stop all containers: %v", err)
	}
	if len(eMap) > 0 {
		e := fmt.Errorf("some container failed during stopping %#v", eMap)
		p.Log(ERROR, eMap)
		if err == nil {
			err = e
		}
	}

	return err
}

func (p *XPod) stopContainers(cList []string, cMap map[string]bool, graceful int) (error, map[string]error) {
	type containerException struct {
		id  string
		err error
	}

	p.Log(INFO, "begin stop containers %s", cList)
	resChan := p.sandbox.WaitProcess(true, cList, graceful)
	errChan := make(chan *containerException, 1)

	for _, cid := range cList {
		c, ok := p.containers[cid]
		if !ok {
			p.Log(WARNING, "no container %s to be stopped", cid)
			delete(cMap, cid)
			continue
		}
		if !c.IsRunning() {
			c.Log(DEBUG, "container state %v is not running(3), ignore during stop", c.CurrentState())
			delete(cMap, cid)
			continue
		}
		go func(c *Container) {
			c.Log(DEBUG, "now, stop container")
			err := c.terminate()
			if err != nil {
				errChan <- &containerException{
					id:  c.Id(),
					err: err,
				}
			}
		}(c)
	}

	if len(cMap) > 0 && resChan == nil {
		err := fmt.Errorf("cannot wait containers %v", cList)
		p.Log(ERROR, err)
		return err, nil
	}

	timeout := utils.Timeout(graceful)
	errMap := map[string]error{}

	for len(cMap) > 0 {
		select {
		case ex, ok := <-resChan:
			if !ok {
				err := fmt.Errorf("chan broken while waiting containers: %#v", cMap)
				p.Log(WARNING, err)
				break //break the select
			}
			p.Log(DEBUG, "container %s stopped (%v)", ex.Id, ex.Code)
			if _, exist := errMap[ex.Id]; exist { //if it exited, ignore the exceptions
				delete(errMap, ex.Id)
			}
			delete(cMap, ex.Id)
		case ex := <-errChan:
			if cMap[ex.id] { //if not waited (maybe already exit, ignore the exception)
				p.Log(WARNING, "fail during killing container %s: %v", ex.id, ex.err)
				errMap[ex.id] = ex.err
				delete(cMap, ex.id)
			}
		case <-timeout:
			err := fmt.Errorf("timeout while waiting containers: %#v of [%v]", cMap, cList)
			p.Log(ERROR, err)
			return err, errMap
		}
	}

	p.Log(INFO, "complete stop containers %s", cList)
	return nil, errMap
}

func (p *XPod) waitStopDone(timeout int, comments string) bool {
	select {
	case s, ok := <-p.stoppedChan:
		if ok {
			p.Log(DEBUG, "got stop msg and push it again: %s", comments)
			select {
			case p.stoppedChan <- s:
			default:
			}
		}
		p.Log(DEBUG, "wait stop done: %s", comments)
		return true
	case <-utils.Timeout(timeout):
		p.Log(DEBUG, "wait stop timeout: %s", comments)
		return false
	}
}

// waitVMStop() should only be call for the life monitoring, others should wait the `waitStopDone`
func (p *XPod) waitVMStop() {
	p.statusLock.RLock()
	if p.status == S_POD_STOPPED {
		p.statusLock.RUnlock()
		return
	}
	p.statusLock.RUnlock()

	_, _ = <-p.sandbox.WaitVm(-1)
	p.Log(INFO, "got vm exit event")
	p.cleanup()
}

//cleanup is used to cleanup the resource after VM shutdown. This method should only called by waitVMStop
func (p *XPod) cleanup() {
	//if removing, the remove will get the resourceLock in advance, and it will set
	// the pod status to NONE when it complete.
	// Therefore, if get into cleanup() after remove, cleanup should exit when it
	// got a
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	p.statusLock.RLock()
	if p.status == S_POD_STOPPED || p.status == S_POD_NONE {
		p.statusLock.RUnlock()
		return
	} else {
		p.status = S_POD_STOPPING
	}
	p.statusLock.RUnlock()

	err := p.decommissionResources()
	if err != nil {
		// even if error, we set the vm to be stopped
		p.Log(ERROR, "pod stopping failed, failed to decommit the resources: %v", err)
	}

	p.Log(DEBUG, "tag pod as stopped")
	p.statusLock.Lock()
	if p.status != S_POD_NONE {
		p.status = S_POD_STOPPED
	}
	p.statusLock.Unlock()

	p.Log(INFO, "pod stopped")
	select {
	case p.stoppedChan <- true:
	default:
	}
}

func (p *XPod) decommissionResources() (err error) {
	p.Log(DEBUG, "umount all containers and volumes, release IP addresses")

	for _, c := range p.containers {
		ec := c.umountRootVol()
		if ec != nil {
			err = ec
			c.Log(ERROR, err)
		}
	}

	for _, v := range p.volumes {
		ev := v.umount()
		if ev != nil {
			err = ev
			v.Log(ERROR, err)
		}
	}

	for _, n := range p.interfaces {
		ei := n.cleanup()
		if ei != nil {
			err = ei
			n.Log(ERROR, err)
		}
	}

	cleanupHosts(p.Id())
	// then it could be start again.
	p.factory.hosts = HostsCreator(p.Id())

	return err
}
