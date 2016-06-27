package daemon

import (
	"fmt"
	"io"

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

	if execId != "" {
		if es := pod.Status().GetExec(execId); es != nil {
			return int(es.ExitCode), nil
		}
		return -1, fmt.Errorf("cannot find exec %s", execId)
	}

	if cs := pod.Status().GetContainer(containerId); cs != nil {
		return int(cs.ExitCode), nil
	}

	return -1, fmt.Errorf("cannot find container %s", containerId)
}

func (daemon *Daemon) Exec(stdin io.ReadCloser, stdout io.WriteCloser, key, id, cmd, tag string, terminal bool) error {
	var (
		vmId      string
		container string
		err       error
	)

	tty := &hypervisor.TtyIO{
		ClientTag: tag,
		Stdin:     stdin,
		Stdout:    stdout,
		Callback:  make(chan *types.VmResponse, 1),
	}

	execId := fmt.Sprintf("exec-%s", utils.RandStr(10, "alpha"))

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

		container = id

		vmId, err = daemon.GetVmByPodId(pod.Id)
		if err != nil {
			return err
		}

		pod.Status().AddExec(container, execId, cmd)

		defer func() {
			if err != nil {
				pod.Status().DeleteExec(execId)
			}
		}()
	}

	vm, ok := daemon.VmList.Get(vmId)
	if !ok {
		err = fmt.Errorf("Can not find VM whose Id is %s!", vmId)
		return err
	}

	if err := vm.Exec(container, execId, cmd, terminal, tty); err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
