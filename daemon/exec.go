package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (daemon *Daemon) ExitCode(container, tag string) (int, error) {
	glog.V(1).Infof("Get container id is %s", container)

	pod, _, err := daemon.GetPodByContainerIdOrName(container)
	if err != nil {
		return -1, err
	}

	pod.RLock()
	defer pod.RUnlock()

	if exec, ok := pod.execList[tag]; ok {
		delete(pod.execList, tag)
		return int(exec.ExitCode), nil
	}

	for _, c := range pod.ctnInfo {
		if c.Id == container {
			return int(c.ExitCode), nil
		}
	}

	return -1, fmt.Errorf("Tag %s incorrect", tag)
}

func (daemon *Daemon) Exec(stdin io.ReadCloser, stdout io.WriteCloser, key, id, cmd, tag string, terminal bool) error {
	var (
		vmId      string
		container string
		err       error
	)

	execId := fmt.Sprintf("exec-%s", pod.RandStr(10, "alpha"))
	exec := &hypervisor.ExecInfo{
		ExecId:    execId,
		ClientTag: execId,
		Command:   cmd,
		ExitCode:  255,
		Terminal:  terminal,
		Stdin:     stdin,
		Stdout:    stdout,
	}

	// We need find the vm id which running POD, and stop it
	if key == "pod" {
		vmId = id
		container = ""
	} else {
		glog.V(1).Infof("Get container id is %s", id)
		pod, _, err := daemon.GetPodByContainerIdOrName(id)
		if err != nil {
			return err
		}

		exec.Container = id

		pod.Lock()
		pod.execList[tag] = exec
		pod.Unlock()

		defer func() {
			if err != nil {
				pod.Lock()
				delete(pod.execList, tag)
				pod.Unlock()
			}
		}()

		vmId, err = daemon.GetVmByPodId(pod.Id)
		if err != nil {
			return err
		}
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		err = fmt.Errorf("Can not find VM whose Id is %s!", vmId)
		return err
	}

	if err := vm.Exec(exec); err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
