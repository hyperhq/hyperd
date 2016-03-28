package daemon

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) PodStopped(podId string) {
	// find the vm id which running POD, and stop it
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return
	}

	pod.status.Vm = ""
	if pod.vm == nil {
		return
	}

	daemon.db.DeleteVMByPod(podId)
	daemon.RemoveVm(pod.vm.Id)
	pod.vm = nil
}

func (daemon *Daemon) StopPod(podId, stopVm string) (int, string, error) {

	return daemon.StopPodWithLock(podId, stopVm)
}

func (daemon *Daemon) StopPodWithLock(podId, stopVm string) (int, string, error) {
	glog.Infof("Prepare to stop the POD: %s", podId)
	// find the vm id which running POD, and stop it
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return -1, "", fmt.Errorf("Can not find pod(%s)", podId)
	}
	// we need to set the 'RestartPolicy' of the pod to 'never' if stop command is invoked
	// for kubernetes
	if pod.status.Type == "kubernetes" {
		pod.status.RestartPolicy = "never"
	}

	if pod.vm == nil {
		return types.E_VM_SHUTDOWN, "", nil
	}

	vmId := pod.vm.Id
	vmResponse := pod.vm.StopPod(pod.status, stopVm)

	// Delete the Vm info for POD
	daemon.db.DeleteVMByPod(podId)

	if vmResponse.Code == types.E_VM_SHUTDOWN {
		daemon.RemoveVm(vmId)
	}
	pod.vm = nil

	return vmResponse.Code, vmResponse.Cause, nil
}
