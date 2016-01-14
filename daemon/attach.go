package daemon

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CmdAttach(job *engine.Job) (err error) {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'attach' command without any container/pod ID!")
	}
	if len(job.Args) == 1 {
		return fmt.Errorf("Can not execute 'attach' command without any command!")
	}
	typeKey := job.Args[0]
	typeVal := job.Args[1]
	tag := job.Args[2]

	var podId, container string

	// We need find the vm id which running POD, and stop it
	if typeKey == "pod" {
		podId = typeVal
		container = ""
	} else {
		container = typeVal
		podId, err = daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
	}

	vmId, err := daemon.GetVmByPodId(podId)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can find VM whose Id is %s!", vmId)
	}

	ttyCallback := make(chan *types.VmResponse, 1)
	err = vm.Attach(job.Stdin, job.Stdout, tag, container, ttyCallback, nil)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	vm.GetExitCode(tag, ttyCallback)

	return nil
}
