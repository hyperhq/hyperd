package daemon

import (
	"fmt"
	"syscall"
	"time"

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

	daemon.RemoveVm(pod.VM.Id)

	pod.Cleanup(daemon)
}

func (daemon *Daemon) PodWait(podId string) {
	// find the vm id which running POD, and stop it
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return
	}

	// wait until PodStopped() was called
	for {
		pod.Lock()
		if pod.VM == nil {
			break
		}
		pod.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	pod.Unlock()
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
	if pod.PodStatus.Type == "kubernetes" {
		pod.PodStatus.RestartPolicy = "never"
	}

	pod.Lock()
	if pod.VM == nil {
		pod.Unlock()
		return types.E_VM_SHUTDOWN, "", nil
	}

	vm := pod.VM

	if pod.PodStatus.Status != types.S_POD_RUNNING {
		pod.Unlock()
		glog.Errorf("Pod %s is not in running state, cannot be stopped", pod.Id)
		return -1, "", fmt.Errorf("Pod %s is not in running state, cannot be stopped", pod.Id)
	}

	pod.Unlock()

	vmResponse := vm.StopPod(pod.PodStatus)

	return vmResponse.Code, vmResponse.Cause, nil
}

func (daemon *Daemon) StopContainer(container string) error {
	pod, idx, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		return fmt.Errorf("can not found container %s", container)
	}

	if !pod.TransitionLock("stop") {
		glog.Errorf("Pod %s is under other operation", pod.Id)
		return fmt.Errorf("Pod %s is under other operation", pod.Id)
	}
	defer pod.TransitionUnlock("stop")

	containerId := pod.PodStatus.Containers[idx].Id
	glog.V(1).Infof("found container %s to stop", containerId)

	return daemon.StopContainerWithinLock(pod, containerId)
}

func (daemon *Daemon) StopContainerWithinLock(pod *Pod, containerId string) error {
	pod.Lock()

	if pod.VM == nil {
		pod.Unlock()
		return fmt.Errorf("pod is not started yet")
	}

	err := pod.VM.KillContainer(containerId, syscall.SIGKILL)
	if err != nil {
		pod.Unlock()
		return err
	}

	pod.Unlock()

	return nil
}
