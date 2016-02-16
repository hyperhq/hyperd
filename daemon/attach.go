package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) Attach(stdin io.ReadCloser, stdout io.WriteCloser, key, id, tag string) error {
	var podId, vmId, container string
	var err error

	// We need find the vm id which running POD, and stop it
	if key == "pod" {
		podId = id
		container = ""
	} else {
		container = id
		podId, err = daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
	}

	vmId, err = daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can find VM whose Id is %s!", vmId)
	}

	ttyCallback := make(chan *types.VmResponse, 1)
	err = vm.Attach(stdin, stdout, tag, container, ttyCallback, nil)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	vm.GetExitCode(tag, ttyCallback)

	return nil
}
