package daemon

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CmdPodStop(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'stop' command without any pod name!")
	}
	podId := job.Args[0]
	stopVm := job.Args[1]
	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()
	code, cause, err := daemon.StopPod(podId, stopVm)
	if err != nil {
		return err
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) PodStopped(podId string) {
	// find the vm id which running POD, and stop it
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return
	}

	if pod.vm == nil {
		return
	}

	daemon.DeleteVmByPod(podId)
	daemon.RemoveVm(pod.vm.Id)
	if pod.status.Autoremove == true {
		daemon.CleanPod(podId)
	}
	pod.vm = nil
}

func (daemon *Daemon) StopPod(podId, stopVm string) (int, string, error) {
	glog.V(1).Infof("Prepare to stop the POD: %s", podId)
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
	daemon.DeleteVmByPod(podId)

	if vmResponse.Code == types.E_VM_SHUTDOWN {
		daemon.RemoveVm(vmId)
	}
	if pod.status.Autoremove == true {
		daemon.CleanPod(podId)
	}
	pod.vm = nil
	return vmResponse.Code, vmResponse.Cause, nil
}
