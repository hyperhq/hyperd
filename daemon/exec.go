package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) ExitCode(container, tag string) (int, error) {
	glog.V(1).Infof("Get container id is %s", container)

	pod, _, err := daemon.GetPodByContainerIdOrName(container)
	if err != nil {
		return -1, err
	}

	pod.RLock()
	tty, ok := pod.ttyList[tag]
	defer pod.RUnlock()

	if !ok {
		return -1, fmt.Errorf("Tag %s incorrect", tag)
	}

	delete(pod.ttyList, tty.ClientTag)

	return int(tty.ExitCode), nil
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

	if err := vm.Exec(container, cmd, terminal, tty); err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
