package daemon

import (
	"fmt"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdExec(job *engine.Job) (err error) {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'exec' command without any container ID!")
	}
	if len(job.Args) == 1 {
		return fmt.Errorf("Can not execute 'exec' command without any command!")
	}
	var (
		typeKey   = job.Args[0]
		typeVal   = job.Args[1]
		cmd       = job.Args[2]
		tag       = job.Args[3]
		vmId      string
		podId     string
		container string
	)

	// We need find the vm id which running POD, and stop it
	if typeKey == "pod" {
		vmId = typeVal
		container = ""
	} else {
		container = typeVal
		glog.V(1).Infof("Get container id is %s", container)
		podId, err = daemon.GetPodByContainer(container)
		if err != nil {
			return
		}
		vmId, err = daemon.GetPodVmByName(podId)
	}

	if err != nil {
		return err
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return fmt.Errorf("Can not find VM whose Id is %s!", vmId)
	}

	err = vm.Exec(job.Stdin, job.Stdout, cmd, tag, container)

	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()
	return nil
}
