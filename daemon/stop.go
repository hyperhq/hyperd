package daemon

import (
	"fmt"
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

	defer pod.wg.Done()

	pod.Lock()
	defer pod.Unlock()

	daemon.RemoveVm(pod.vm.Id)

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
		if pod.vm == nil {
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
	if pod.status.Type == "kubernetes" {
		pod.status.RestartPolicy = "never"
	}

	pod.Lock()

	if pod.vm == nil {
		pod.Unlock()
		return types.E_VM_SHUTDOWN, "", nil
	}

	if pod.status.Status != types.S_POD_RUNNING {
		pod.Unlock()
		glog.Errorf("Pod %s is not in running state, cannot be stopped", pod.Id)
		return -1, "", fmt.Errorf("Pod %s is not in running state, cannot be stopped", pod.Id)
	}

	vmResponse := pod.vm.StopPod(pod.status)
	pod.Unlock()
	pod.wg.Wait()
	return vmResponse.Code, vmResponse.Cause, nil
}
