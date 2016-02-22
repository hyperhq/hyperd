package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
)

func (daemon *Daemon) ExitCode(container, tag string) (int, error) {
	glog.V(1).Infof("Get container id is %s", container)
	podId, err := daemon.GetPodByContainer(container)
	if err != nil {
		return -1, err
	}

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return -1, err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return -1, fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	if _, ok := vm.ExitCodes[tag]; !ok {
		return -1, fmt.Errorf("Tag %s incorrect", tag)
	}

	return int(vm.ExitCodes[tag]), nil
}

func (daemon *Daemon) Exec(stdin io.ReadCloser, stdout io.WriteCloser, key, id, cmd, tag string) error {
	var (
		vmId      string
		container string
	)

	// We need find the vm id which running POD, and stop it
	if key == "pod" {
		vmId = id
		container = ""
	} else {
		container = id
		glog.V(1).Infof("Get container id is %s", container)
		podId, err := daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
		vmId, err = daemon.GetVmByPodId(podId)
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	if err := vm.Exec(stdin, stdout, cmd, tag, container); err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
