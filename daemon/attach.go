package daemon

import (
	"fmt"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/lib/glog"
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

	var podName, container string

	// We need find the vm id which running POD, and stop it
	if typeKey == "pod" {
		podName = typeVal
		container = ""
	} else {
		container = typeVal
		podName, err = daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
	}

	vmId, err := daemon.GetPodVmByName(podName)
	if err != nil {
		return err
	}

	vm, ok := daemon.vmList[vmId]
	if !ok {
		return fmt.Errorf("Can find VM whose Id is %s!", vmId)
	}

	err = vm.Attach(job.Stdin, job.Stdout, tag, container)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()

	return nil
}
