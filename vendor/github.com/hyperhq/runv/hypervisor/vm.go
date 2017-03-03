package hypervisor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
)

type Vm struct {
	Id string

	ctx *VmContext

	//Pod    *PodStatus
	//Status uint
	Cpu  int
	Mem  int
	Lazy bool

	clients *Fanout
}

func (vm *Vm) GetResponseChan() (chan *types.VmResponse, error) {
	if vm.clients != nil {
		return vm.clients.Acquire()
	}
	return nil, errors.New("No channels available")
}

func (vm *Vm) ReleaseResponseChan(ch chan *types.VmResponse) {
	if vm.clients != nil {
		vm.clients.Release(ch)
	}
}

func (vm *Vm) Launch(b *BootConfig) (err error) {
	var (
		vmEvent = make(chan VmEvent, 128)
		Status  = make(chan *types.VmResponse, 128)
		ctx     *VmContext
	)

	ctx, err = InitContext(vm.Id, vmEvent, Status, nil, b)
	if err != nil {
		Status <- &types.VmResponse{
			VmId:  vm.Id,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return err

	}

	ctx.Launch()
	vm.ctx = ctx

	vm.clients = CreateFanout(Status, 128, false)

	return nil
}

// This function will only be invoked during daemon start
func AssociateVm(vmId string, data []byte) (*Vm, error) {
	var (
		PodEvent = make(chan VmEvent, 128)
		Status   = make(chan *types.VmResponse, 128)
		err      error
	)

	vm := newVm(vmId, 0, 0)
	vm.ctx, err = VmAssociate(vm.Id, PodEvent, Status, data)
	if err != nil {
		glog.Errorf("cannot associate with vm: %v", err)
		return nil, err
	}

	vm.clients = CreateFanout(Status, 128, false)
	return vm, nil
}

type matchResponse func(response *types.VmResponse) (error, bool)

func (vm *Vm) WaitResponse(match matchResponse, timeout int) chan error {
	result := make(chan error, 1)
	var timeoutChan <-chan time.Time
	if timeout >= 0 {
		timeoutChan = time.After(time.Duration(timeout) * time.Second)
	} else {
		timeoutChan = make(chan time.Time, 1)
	}

	Status, err := vm.GetResponseChan()
	if err != nil {
		result <- err
		return result
	}
	go func() {
		defer vm.ReleaseResponseChan(Status)

		for {
			select {
			case response, ok := <-Status:
				if !ok {
					result <- fmt.Errorf("Response Chan is broken")
					return
				}
				if err, exit := match(response); exit {
					result <- err
					return
				}
			case <-timeoutChan:
				result <- fmt.Errorf("timeout for waiting response")
				return
			}
		}
	}()
	return result
}

func (vm *Vm) ReleaseVm() error {
	if vm.ctx.current != StateRunning {
		return nil
	}

	result := vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
		if response.Code == types.E_VM_SHUTDOWN || response.Code == types.E_OK {
			return nil, true
		}
		return nil, false
	}, -1)

	releasePodEvent := &ReleaseVMCommand{}

	if err := vm.ctx.SendVmEvent(releasePodEvent); err != nil {
		return err
	}

	return <-result
}

func (vm *Vm) WaitVm(timeout int) <-chan error {
	return vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
		if response.Code == types.E_VM_SHUTDOWN {
			return nil, true
		}
		return nil, false
	}, timeout)
}

func (vm *Vm) WaitProcess(isContainer bool, ids []string, timeout int) <-chan *api.ProcessExit {
	var (
		waiting   = make(map[string]struct{})
		result    = make(chan *api.ProcessExit, len(ids))
		waitEvent = types.E_CONTAINER_FINISHED
	)

	if !isContainer {
		waitEvent = types.E_EXEC_FINISHED
	}

	for _, id := range ids {
		waiting[id] = struct{}{}
	}

	resChan := vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
		if response.Code == types.E_VM_SHUTDOWN {
			return fmt.Errorf("get shutdown event"), true
		}
		if response.Code != waitEvent {
			return nil, false
		}
		ps, _ := response.Data.(*types.ProcessFinished)
		if _, ok := waiting[ps.Id]; ok {
			result <- &api.ProcessExit{
				Id:         ps.Id,
				Code:       int(ps.Code),
				FinishedAt: time.Now().UTC(),
			}
			select {
			case ps.Ack <- true:
				vm.ctx.Log(TRACE, "got shut down msg, acked here")
			default:
				vm.ctx.Log(TRACE, "got shut down msg, acked somewhere")
			}
			delete(waiting, ps.Id)
			if len(waiting) == 0 {
				// got all of processexit event, exit
				return nil, true
			}
		}
		// continue to wait other processexit event
		return nil, false
	}, timeout)

	go func() {
		if err := <-resChan; err != nil {
			close(result)
		}
	}()

	return result
}

//func (vm *Vm) handlePodEvent(mypod *PodStatus) {
//	glog.V(1).Infof("hyperHandlePodEvent pod %s, vm %s", mypod.Id, vm.Id)
//
//	Status, err := vm.GetResponseChan()
//	if err != nil {
//		return
//	}
//	defer vm.ReleaseResponseChan(Status)
//
//	exit := false
//	mypod.Wg.Add(1)
//	for {
//		Response, ok := <-Status
//		if !ok {
//			break
//		}
//
//		switch Response.Code {
//		case types.E_CONTAINER_FINISHED:
//			ps, ok := Response.Data.(*types.ProcessFinished)
//			if ok {
//				mypod.SetOneContainerStatus(ps.Id, ps.Code)
//				close(ps.Ack)
//			}
//		case types.E_EXEC_FINISHED:
//			ps, ok := Response.Data.(*types.ProcessFinished)
//			if ok {
//				mypod.SetExecStatus(ps.Id, ps.Code)
//				close(ps.Ack)
//			}
//		case types.E_VM_SHUTDOWN: // vm exited, sucessful or not
//			if mypod.Status == types.S_POD_RUNNING { // not received finished pod before
//				mypod.Status = types.S_POD_FAILED
//				mypod.FinishedAt = time.Now().Format("2006-01-02T15:04:05Z")
//				mypod.SetContainerStatus(types.S_POD_FAILED)
//			}
//			mypod.Vm = ""
//			exit = true
//		}
//
//		if mypod.Handler != nil {
//			mypod.Handler.Handle(Response, mypod.Handler.Data, mypod, vm)
//		}
//
//		if exit {
//			vm.clients = nil
//			break
//		}
//	}
//	mypod.Wg.Done()
//}

func (vm *Vm) InitSandbox(config *api.SandboxConfig) {
	vm.ctx.SetNetworkEnvironment(config)
	vm.ctx.startPod()
}

func (vm *Vm) WaitInit() api.Result {
	if err := <-vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
		if response.Code == types.E_OK {
			return nil, true
		}
		if response.Code == types.E_FAILED || response.Code == types.E_VM_SHUTDOWN {
			return fmt.Errorf("got failed event when wait init message"), true
		}
		return nil, false
	}, -1); err != nil {
		return api.NewResultBase(vm.Id, false, err.Error())
	}
	return api.NewResultBase(vm.Id, true, "wait init message successfully")
}

func (vm *Vm) Shutdown() api.Result {
	if vm.ctx.current != StateRunning {
		return api.NewResultBase(vm.Id, false, "not in running state")
	}

	result := vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
		if response.Code == types.E_VM_SHUTDOWN {
			return nil, true
		}
		return nil, false
	}, -1)

	if err := vm.ctx.SendVmEvent(&ShutdownCommand{}); err != nil {
		return api.NewResultBase(vm.Id, false, "vm context already exited")
	}

	if err := <-result; err != nil {
		return api.NewResultBase(vm.Id, false, err.Error())
	}
	return api.NewResultBase(vm.Id, true, "shutdown vm successfully")
}

// TODO: should we provide a method to force kill vm
func (vm *Vm) Kill() {
	vm.ctx.poweroffVM(false, "vm.Kill()")
}

func (vm *Vm) WriteFile(container, target string, data []byte) error {
	return vm.ctx.hyperstart.WriteFile(container, target, data)
}

func (vm *Vm) ReadFile(container, target string) ([]byte, error) {
	return vm.ctx.hyperstart.ReadFile(container, target)
}

func (vm *Vm) SignalProcess(container, process string, signal syscall.Signal) error {
	return vm.ctx.hyperstart.SignalProcess(container, process, signal)
}

func (vm *Vm) KillContainer(container string, signal syscall.Signal) error {
	return vm.SignalProcess(container, "init", signal)
}

func (vm *Vm) AddRoute() error {
	routes := vm.ctx.networks.getRoutes()
	return vm.ctx.hyperstart.AddRoute(routes)
}

func (vm *Vm) AddNic(info *api.InterfaceDescription) error {
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	client := make(chan api.Result, 1)
	vm.ctx.AddInterface(info, client)

	ev, ok := <-client
	if !ok {
		return fmt.Errorf("internal error")
	}

	if !ev.IsSuccess() {
		return fmt.Errorf("allocate device failed")
	}

	if vm.ctx.LogLevel(TRACE) {
		glog.Infof("finial vmSpec.Interface is %#v", vm.ctx.networks.getInterface(info.Id))
	}
	return vm.ctx.updateInterface(info.Id)
}

func (vm *Vm) DeleteNic(id string) error {
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	client := make(chan api.Result, 1)
	vm.ctx.RemoveInterface(id, client)

	ev, ok := <-client
	if !ok {
		return fmt.Errorf("internal error")
	}

	if !ev.IsSuccess() {
		return fmt.Errorf("remove device failed")
	}
	return nil
}

// TODO: deprecated api, it will be removed after the hyper.git updated
func (vm *Vm) AddCpu(totalCpu int) error {
	return vm.SetCpus(totalCpu)
}

func (vm *Vm) SetCpus(cpus int) error {
	if vm.Cpu >= cpus {
		return nil
	}

	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	err := vm.ctx.DCtx.SetCpus(vm.ctx, cpus)
	if err == nil {
		vm.Cpu = cpus
	}
	return err
}

func (vm *Vm) AddMem(totalMem int) error {
	if vm.Mem >= totalMem {
		return nil
	}

	size := totalMem - vm.Mem
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	err := vm.ctx.DCtx.AddMem(vm.ctx, 1, size)
	if err == nil {
		vm.Mem = totalMem
	}
	return err
}

func (vm *Vm) OnlineCpuMem() error {
	return vm.ctx.hyperstart.OnlineCpuMem()
}

func (vm *Vm) HyperstartExecSync(cmd []string, stdin []byte) (stdout, stderr []byte, err error) {
	if len(cmd) == 0 {
		return nil, nil, fmt.Errorf("'hyperstart-exec' without command")
	}

	execId := fmt.Sprintf("hyperstart-exec-%s", utils.RandStr(10, "alpha"))

	var stdoutBuf, stderrBuf bytes.Buffer
	tty := &TtyIO{
		Stdin:  ioutil.NopCloser(bytes.NewReader(stdin)),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}

	result := vm.WaitProcess(false, []string{execId}, -1)
	if result == nil {
		err = fmt.Errorf("can not wait hyperstart-exec %q", execId)
		glog.Error(err)
		return nil, nil, err
	}

	err = vm.AddProcess(hyperstartapi.HYPERSTART_EXEC_CONTAINER, execId, false, cmd, []string{}, "/", tty)
	if err != nil {
		return nil, nil, err
	}

	r, ok := <-result
	if !ok {
		err = fmt.Errorf("wait hyperstart-exec %q interrupted", execId)
		glog.Error(err)
		return nil, nil, err
	}

	glog.V(3).Infof("hyperstart-exec %q terminated at %v with code %d", execId, r.FinishedAt, r.Code)

	if r.Code != 0 {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("exit with error code:%d", r.Code)
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

func (vm *Vm) Exec(container, execId, cmd string, terminal bool, tty *TtyIO) error {
	var command []string

	if cmd == "" {
		return fmt.Errorf("'exec' without command")
	}

	if err := json.Unmarshal([]byte(cmd), &command); err != nil {
		return err
	}
	return vm.AddProcess(container, execId, terminal, command, []string{}, "/", tty)
}

func (vm *Vm) AddProcess(container, execId string, terminal bool, args []string, env []string, workdir string, tty *TtyIO) error {
	envs := []hyperstartapi.EnvironmentVar{}

	for _, v := range env {
		if eqlIndex := strings.Index(v, "="); eqlIndex > 0 {
			envs = append(envs, hyperstartapi.EnvironmentVar{
				Env:   v[:eqlIndex],
				Value: v[eqlIndex+1:],
			})
		}
	}

	stdinPipe, stdoutPipe, stderrPipe, err := vm.ctx.hyperstart.AddProcess(container, &hyperstartapi.Process{
		Id:       execId,
		Terminal: terminal,
		Args:     args,
		Envs:     envs,
		Workdir:  workdir,
	})

	if err != nil {
		return fmt.Errorf("exec command %v failed: %v", args, err)
	}

	go streamCopy(tty, stdinPipe, stdoutPipe, stderrPipe)
	return nil
}

func (vm *Vm) AddVolume(vol *api.VolumeDescription) api.Result {
	if vm.ctx.current != StateRunning {
		glog.Errorf("VM is not ready for insert volume %#v", vol)
		return NewNotReadyError(vm.Id)
	}

	result := make(chan api.Result, 1)
	vm.ctx.AddVolume(vol, result)
	return <-result
}

func (vm *Vm) AddContainer(c *api.ContainerDescription) api.Result {
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	result := make(chan api.Result, 1)
	vm.ctx.AddContainer(c, result)
	return <-result
}

func (vm *Vm) RemoveContainer(id string) api.Result {
	result := make(chan api.Result, 1)
	vm.ctx.RemoveContainer(id, result)
	return <-result
}

func (vm *Vm) RemoveVolume(name string) api.Result {
	result := make(chan api.Result, 1)
	vm.ctx.RemoveVolume(name, result)
	return <-result
}

func (vm *Vm) RemoveContainers(ids ...string) (bool, map[string]api.Result) {
	return vm.batchWaitResult(ids, vm.ctx.RemoveContainer)
}

func (vm *Vm) RemoveVolumes(names ...string) (bool, map[string]api.Result) {
	return vm.batchWaitResult(names, vm.ctx.RemoveVolume)
}

type waitResultOp func(string, chan<- api.Result)

func (vm *Vm) batchWaitResult(names []string, op waitResultOp) (bool, map[string]api.Result) {
	var (
		success = true
		result  = map[string]api.Result{}
		wl      = map[string]struct{}{}
		r       = make(chan api.Result, len(names))
	)

	for _, name := range names {
		if _, ok := wl[name]; !ok {
			wl[name] = struct{}{}
			go op(name, r)
		}
	}

	for len(wl) > 0 {
		rsp, ok := <-r
		if !ok {
			vm.ctx.Log(ERROR, "fail to wait channels for op %v on %v", op, names)
			return false, result
		}
		if !rsp.IsSuccess() {
			vm.ctx.Log(ERROR, "batch op %v on %s is not success: %s", op, rsp.ResultId(), rsp.Message())
			success = false
		}
		vm.ctx.Log(DEBUG, "batch op %v on %s returned: %s", op, rsp.Message())
		if _, ok := wl[rsp.ResultId()]; ok {
			delete(wl, rsp.ResultId())
			result[rsp.ResultId()] = rsp
		}
	}

	return success, result
}

func (vm *Vm) StartContainer(id string) error {
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	err := vm.ctx.newContainer(id)
	if err != nil {
		return fmt.Errorf("Create new container failed: %v", err)
	}

	vm.ctx.Log(DEBUG, "container %s start: done.", id)
	return nil
}

func (vm *Vm) Tty(containerId, execId string, row, column int) error {
	if execId == "" {
		execId = "init"
	}
	return vm.ctx.hyperstart.TtyWinResize(containerId, execId, uint16(row), uint16(column))
}

func (vm *Vm) Attach(tty *TtyIO, container string) error {
	if vm.ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	cmd := &AttachCommand{
		Streams:   tty,
		Container: container,
	}
	return vm.ctx.attachCmd(cmd)
}

func (vm *Vm) Stats() *types.PodStats {
	ctx := vm.ctx

	if ctx.current != StateRunning {
		vm.ctx.Log(WARNING, "could not get stats from non-running pod")
		return nil
	}

	stats, err := ctx.DCtx.Stats(ctx)
	if err != nil {
		vm.ctx.Log(WARNING, "failed to get stats: %v", err)
		return nil
	}
	return stats
}

func (vm *Vm) Pause(pause bool) error {
	ctx := vm.ctx
	if ctx.current != StateRunning {
		return NewNotReadyError(vm.Id)
	}

	command := "Pause"
	pauseState := PauseStatePaused
	if !pause {
		pauseState = PauseStateUnpaused
		command = "Unpause"
	}

	var err error
	ctx.pauseLock.Lock()
	defer ctx.pauseLock.Unlock()
	if ctx.PauseState != pauseState {
		/* FIXME: only support pause whole vm now */
		// should not change pause state inside ctx.DCtx.Pause!
		err = ctx.DCtx.Pause(ctx, pause)
		if err != nil {
			glog.Errorf("%s sandbox failed: %v", command, err)
			return err
		}

		glog.V(3).Infof("sandbox state turn to %s now", command)
		ctx.PauseState = pauseState // change the state.
	}

	return nil
}

func (vm *Vm) Save(path string) error {
	ctx := vm.ctx

	ctx.pauseLock.Lock()
	defer ctx.pauseLock.Unlock()
	if ctx.current != StateRunning || ctx.PauseState != PauseStatePaused {
		return NewNotReadyError(vm.Id)
	}

	return ctx.DCtx.Save(ctx, path)
}

func (vm *Vm) GetIPAddrs() []string {
	ips := []string{}

	if vm.ctx.current != StateRunning {
		glog.Errorf("get pod ip failed: %v", NewNotReadyError(vm.Id))
		return ips
	}

	res := vm.ctx.networks.getIpAddrs()
	ips = append(ips, res...)

	return ips
}

func errorResponse(cause string) *types.VmResponse {
	return &types.VmResponse{
		Code:  -1,
		Cause: cause,
		Data:  nil,
	}
}

func newVm(vmId string, cpu, memory int) *Vm {
	return &Vm{
		Id:  vmId,
		Cpu: cpu,
		Mem: memory,
	}
}

func GetVm(vmId string, b *BootConfig, waitStarted bool) (*Vm, error) {
	id := vmId
	if id == "" {
		for {
			id = fmt.Sprintf("vm-%s", utils.RandStr(10, "alpha"))
			if _, err := os.Stat(BaseDir + "/" + id); os.IsNotExist(err) {
				break
			}
		}
	}

	vm := newVm(id, b.CPU, b.Memory)
	if err := vm.Launch(b); err != nil {
		return nil, err
	}

	if waitStarted {
		glog.V(1).Info("waiting for vm to start")
		if err := <-vm.WaitResponse(func(response *types.VmResponse) (error, bool) {
			if response.Code == types.E_FAILED {
				glog.Error("VM start failed")
				return fmt.Errorf("vm start failed"), true
			}
			if response.Code == types.E_VM_RUNNING {
				glog.V(1).Info("VM started successfully")
				return nil, true
			}
			glog.Error("VM never started")
			return nil, false
		}, -1); err != nil {
			vm.Kill()
		}
	}

	glog.V(1).Info("GetVm succeeded (not waiting for startup)")
	return vm, nil
}
