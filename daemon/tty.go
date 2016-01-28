package daemon

import (
	"fmt"
	"github.com/golang/glog"
	"strings"
)

func (daemon *Daemon) TtyResize(podId, tag string, h, w int) error {
	var (
		container string
		vmid      string
		err       error
	)

	if strings.Contains(podId, "pod-") {
		container = ""
		vmid, err = daemon.GetVmByPodId(podId)
		if err != nil {
			return err
		}
	} else if strings.Contains(podId, "vm-") {
		vmid = podId
	} else {
		container = podId
		podId, err = daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
		vmid, err = daemon.GetVmByPodId(podId)
		if err != nil {
			return err
		}
	}

	vm, ok := daemon.VmList[vmid]
	if !ok {
		return fmt.Errorf("vm %s doesn't exist!")
	}

	err = vm.Tty(tag, h, w)
	if err != nil {
		return err
	}

	glog.V(1).Infof("Success to resize the tty!")
	return nil
}
