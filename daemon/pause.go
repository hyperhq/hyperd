package daemon

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdPause(job *engine.Job) (err error) {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'pause' command without pod id!")
	}

	podId := job.Args[0]

	glog.V(1).Infof("Get pod id is %s", podId)

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	return vm.Pause(true)
}

func (daemon Daemon) PauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	podId, err := daemon.GetPodByContainer(container)
	if err != nil {
		return err
	}

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	return vm.Pause(true)
}

func (daemon *Daemon) CmdUnpause(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'pause' command without pod id!")
	}

	podId := job.Args[0]

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	return vm.Pause(false)
}

func (daemon *Daemon) UnpauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	podId, err := daemon.GetPodByContainer(container)
	if err != nil {
		return err
	}

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	return vm.Pause(false)
}
