package daemon

import (
	"fmt"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdPodStop(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'stop' command without any pod name!")
	}
	podId := job.Args[0]
	stopVm := job.Args[1]
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

func (daemon *Daemon) StopPod(podId, stopVm string) (int, string, error) {
	glog.V(1).Infof("Prepare to stop the POD: %s", podId)
	// find the vm id which running POD, and stop it
	mypod, ok := daemon.PodList[podId]
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return -1, "", fmt.Errorf("Can not find pod(%s)", podId)
	}
	vmid, err := daemon.GetPodVmByName(podId)
	if err != nil {
		return -1, "", err
	}
	// we need to set the 'RestartPolicy' of the pod to 'never' if stop command is invoked
	// for kubernetes
	if mypod.Type == "kubernetes" {
		mypod.RestartPolicy = "never"
		if mypod.Vm == "" {
			return types.E_VM_SHUTDOWN, "", nil
		}
	}

	vm, ok := daemon.VmList[vmid]
	if !ok {
		return -1, "", fmt.Errorf("VM is not exist")
	}

	vmResponse := vm.StopPod(mypod, stopVm)

	// Delete the Vm info for POD
	daemon.DeleteVmByPod(podId)

	if vmResponse.Code == types.E_VM_SHUTDOWN {
		daemon.RemoveVm(vmid)
	}
	if mypod.Autoremove == true {
		daemon.CleanPod(podId)
	}

	return vmResponse.Code, vmResponse.Cause, nil
}
