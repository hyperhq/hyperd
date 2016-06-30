package daemon

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon Daemon) PausePod(podId string) error {
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
	}

	if !pod.TransitionLock("pause") {
		return fmt.Errorf("Pod %s is under other operation, please try again later", podId)
	}
	defer pod.TransitionUnlock("pause")

	vmId := pod.PodStatus.Vm

	vm, ok := daemon.VmList.Get(vmId)
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	if err := vm.Pause(true); err != nil {
		return err
	}

	pod.PodStatus.SetContainerStatus(types.S_POD_PAUSED)
	pod.PodStatus.Status = types.S_POD_PAUSED
	vm.Status = types.S_VM_PAUSED

	return nil
}

func (daemon Daemon) PauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	podId, err := daemon.GetPodByContainer(container)
	if err != nil {
		return err
	}

	return daemon.PausePod(podId)
}

func (daemon *Daemon) UnpausePod(podId string) error {
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
	}

	if !pod.TransitionLock("unpause") {
		return fmt.Errorf("Pod %s is under other operation, please try again later", podId)
	}
	defer pod.TransitionUnlock("unpause")

	vmId := pod.PodStatus.Vm

	if pod.PodStatus.Status != types.S_POD_PAUSED {
		return fmt.Errorf("pod is not paused")
	}

	vm, ok := daemon.VmList.Get(vmId)
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	if err := vm.Pause(false); err != nil {
		return err
	}

	pod.PodStatus.SetContainerStatus(types.S_POD_RUNNING)
	pod.PodStatus.Status = types.S_POD_RUNNING
	vm.Status = types.S_VM_ASSOCIATED

	return nil
}

func (daemon *Daemon) UnpauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	podId, err := daemon.GetPodByContainer(container)
	if err != nil {
		return err
	}

	return daemon.UnpausePod(podId)
}
