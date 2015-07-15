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

	// Prepare the qemu status to client
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
	if daemon.podList[podId].Status != types.S_POD_RUNNING {
		return -1, "", fmt.Errorf("The POD %s has aleady stopped, can not stop again!", podId)
	}
	vmid, err := daemon.GetPodVmByName(podId)
	if err != nil {
		return -1, "", err
	}
	// we need to set the 'RestartPolicy' of the pod to 'never' if stop command is invoked
	// for kubernetes
	if daemon.podList[podId].Type == "kubernetes" {
		daemon.podList[podId].RestartPolicy = "never"
		if daemon.podList[podId].Vm == "" {
			return types.E_VM_SHUTDOWN, "", nil
		}
	}

	vm, _ := daemon.vmList[vmid]
	mypod, _ := daemon.podList[podId]

	qemuResponse := vm.StopPod(mypod, stopVm)

	// Delete the Vm info for POD
	daemon.DeleteVmByPod(podId)

	if qemuResponse.Code == types.E_VM_SHUTDOWN {
		daemon.RemoveVm(vmid)
	}

	return qemuResponse.Code, qemuResponse.Cause, nil
}
