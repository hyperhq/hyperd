package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) Attach(stdin io.ReadCloser, stdout io.WriteCloser, key, id, tag string) error {
	var (
		podId     string
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

	// We need find the vm id which running POD, and stop it
	if key == "pod" {
		podId = id
		container = ""
	} else {
		container = id
		pod, _, err := daemon.GetPodByContainerIdOrName(container)
		if err != nil {
			return err
		}

		podId = pod.Id
		pod.Lock()
		pod.ttyList[tag] = tty
		pod.Unlock()

		defer func() {
			if err != nil && pod != nil {
				pod.Lock()
				delete(pod.ttyList, tag)
				pod.Unlock()
			}
		}()
	}

	vmId, err = daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		err = fmt.Errorf("Can find VM whose Id is %s!", vmId)
		return err
	}

	err = vm.Attach(tty, container, nil)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for attach!")
	}()

	err = tty.WaitForFinish()

	return err
}
