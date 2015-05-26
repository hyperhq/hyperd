package daemon

import (
	"fmt"
	"encoding/json"

	"hyper/engine"
	"hyper/lib/glog"
	"hyper/qemu"
	"hyper/types"
)

func (daemon *Daemon) CmdExec(job *engine.Job) (err error) {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not execute 'exec' command without any container ID!")
	}
	if len(job.Args) == 1 {
		return fmt.Errorf("Can not execute 'exec' command without any command!")
	}
	var (
		typeKey = job.Args[0]
		typeVal = job.Args[1]
		tag = job.Args[3]
		vmId string
		podId string
		command = []string{}
	)

	if job.Args[2] != "" {
		if err = json.Unmarshal([]byte(job.Args[2]), &command); err != nil {
			return err
		}
	}

	// We need find the vm id which running POD, and stop it
	if typeKey == "pod" {
		vmId = typeVal
	} else {
		container := typeVal
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

	execCmd := &qemu.ExecCommand{
		Command: command,
		Streams: &qemu.TtyIO{
			Stdin: job.Stdin,
			Stdout: job.Stdout,
			ClientTag: tag,
			Callback: make(chan *types.QemuResponse, 1),
		},
	}

	if typeKey == "pod" {
		execCmd.Container = ""
	} else {
		execCmd.Container = typeVal
	}

	qemuEvent, _, _,err := daemon.GetQemuChan(vmId)
	if err != nil {
		return err
	}

	qemuEvent.(chan qemu.QemuEvent) <- execCmd

	<-execCmd.Streams.Callback
	defer func() {
		glog.V(2).Info("Defer function for exec!")
	} ()
	return nil
}
