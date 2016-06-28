package daemon

import (
	"fmt"
	"github.com/golang/glog"
)

func (daemon *Daemon) TtyResize(containerId, execId string, h, w int) error {
	podId, err := daemon.GetPodByContainer(containerId)
	if err != nil {
		return err
	}
	vmid, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList.Get(vmid)
	if !ok {
		return fmt.Errorf("vm %s doesn't exist!", vmid)
	}

	err = vm.Tty(containerId, execId, h, w)
	if err != nil {
		return err
	}

	glog.V(1).Infof("Success to resize the tty!")
	return nil
}
