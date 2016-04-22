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

	pod.Lock()
	defer pod.Unlock()

	pod.status.Vm = ""
	if pod.vm == nil {
		return
	}

	pod.cleanupEtcHosts()
	daemon.db.DeleteVMByPod(podId)
	daemon.RemoveVm(pod.vm.Id)
	pod.vm = nil

	if pod.status.Status == types.S_POD_NONE {
		daemon.RemovePodResource(pod)
	}
}

func (daemon *Daemon) StopPod(podId string) (int, string, error) {
	glog.Infof("Prepare to stop the POD: %s", podId)
	// find the vm id which running POD, and stop it
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return -1, "", fmt.Errorf("Can not find pod(%s)", podId)
	}

	if !pod.TransitionLock("stop") {
		glog.Errorf("Pod %s is under other operation", podId)
		return -1, "", fmt.Errorf("Pod %s is under other operation", podId)
	}
	defer pod.TransitionUnlock("stop")

	return daemon.StopPodWithinLock(pod)
}

func (daemon *Daemon) StopPodWithinLock(pod *Pod) (int, string, error) {
	// we need to set the 'RestartPolicy' of the pod to 'never' if stop command is invoked
	// for kubernetes
	if pod.status.Type == "kubernetes" {
		pod.status.RestartPolicy = "never"
	}

	pod.Lock()
	defer pod.Unlock()

	if pod.vm == nil {
		return types.E_VM_SHUTDOWN, "", nil
	}

	if pod.status.Status != types.S_POD_RUNNING {
		glog.Errorf("Pod %s is not in running state, cannot be stopped", pod.id)
		return -1, "", fmt.Errorf("Pod %s is not in running state, cannot be stopped", pod.id)
	}

	vmResponse := pod.vm.StopPod(pod.status)

	return vmResponse.Code, vmResponse.Cause, nil
}
