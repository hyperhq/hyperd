package daemon

import (
	"fmt"
	"hyper/engine"
	"hyper/lib/glog"
	"hyper/hypervisor"
	"hyper/types"
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
	var podName string

	// We need find the vm id which running POD, and stop it
	if typeKey == "pod" {
		podName = typeVal
	} else {
		container := typeVal
		podName, err = daemon.GetPodByContainer(container)
		if err != nil {
			return
		}
	}
	vmid, err := daemon.GetPodVmByName(podName)
	if err != nil {
		return err
	}
	var (
		ttyIO        hypervisor.TtyIO
		qemuCallback = make(chan *types.QemuResponse, 1)
	)

	ttyIO.Stdin = job.Stdin
	ttyIO.Stdout = job.Stdout
	ttyIO.ClientTag = tag
	ttyIO.Callback = qemuCallback

	var attachCommand = &hypervisor.AttachCommand{
		Streams: &ttyIO,
		Size:    nil,
	}
	if typeKey == "pod" {
		attachCommand.Container = ""
	} else {
		attachCommand.Container = typeVal
	}
	qemuEvent, _, _, err := daemon.GetQemuChan(vmid)
	if err != nil {
		return err
	}
	qemuEvent.(chan hypervisor.QemuEvent) <- attachCommand

	<-qemuCallback
	defer func() {
		glog.V(2).Info("Defer function for exec!")
	}()
	return nil
}
