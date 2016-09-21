package daemon

import (
	"fmt"
	"io"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/golang/glog"

	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) ExitCode(containerId, execId string) (int, error) {
	glog.V(1).Infof("Get container id %s, exec id %s", containerId, execId)

	pod, _, err := daemon.GetPodByContainerIdOrName(containerId)
	if err != nil {
		return -1, err
	}

	status := pod.Status()
	if status == nil {
		return -1, fmt.Errorf("cannot find status of pod %s", pod.Id)
	}

	if execId != "" {
		if es := status.GetExec(execId); es != nil {
			return int(es.ExitCode), nil
		}
		return -1, fmt.Errorf("cannot find exec %s", execId)
	}

	if cs := status.GetContainer(containerId); cs != nil {
		return int(cs.ExitCode), nil
	}

	return -1, fmt.Errorf("cannot find container %s", containerId)
}

func (daemon *Daemon) CreateExec(containerId, cmd string, terminal bool) (string, error) {
	execId := fmt.Sprintf("exec-%s", utils.RandStr(10, "alpha"))

	glog.V(1).Infof("Get container id is %s", containerId)
	pod, _, err := daemon.GetPodByContainerIdOrName(containerId)
	if err != nil {
		return "", err
	}

	status := pod.Status()
	if status == nil || status.Status != types.S_POD_RUNNING {
		return "", fmt.Errorf("container %s is not running", containerId)
	}

	status.AddExec(containerId, execId, cmd, terminal)
	return execId, nil
}

func (daemon *Daemon) StartExec(stdin io.ReadCloser, stdout io.WriteCloser, containerId, execId string) error {
	tty := &hypervisor.TtyIO{
		Stdin:    stdin,
		Stdout:   stdout,
		Callback: make(chan *types.VmResponse, 1),
	}

	glog.V(1).Infof("Get container id is %s", containerId)
	pod, _, err := daemon.GetPodByContainerIdOrName(containerId)
	if err != nil {
		return err
	}

	status := pod.Status()
	if status == nil || status.Status != types.S_POD_RUNNING {
		return fmt.Errorf("container %s is not running", containerId)
	}

	es := status.GetExec(execId)
	if es == nil {
		return fmt.Errorf("Can not find exec %s", execId)
	}

	vmId, err := daemon.GetVmByPodId(pod.Id)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList.Get(vmId)
	if !ok {
		err = fmt.Errorf("Can not find VM whose Id is %s!", vmId)
		return err
	}

	if !es.Terminal {
		tty.Stderr = stdcopy.NewStdWriter(stdout, stdcopy.Stderr)
		tty.Stdout = stdcopy.NewStdWriter(stdout, stdcopy.Stdout)
		tty.OutCloser = stdout
	}

	if err := vm.Exec(es.Container, es.Id, es.Cmds, es.Terminal, tty); err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
