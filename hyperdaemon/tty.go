package daemon

import (
	"hyper/engine"
	"hyper/hypervisor"
	"hyper/lib/glog"
	"strconv"
	"strings"
)

func (daemon *Daemon) CmdTty(job *engine.Job) (err error) {
	if len(job.Args) < 3 {
		return nil
	}
	var (
		podID     = job.Args[0]
		tag       = job.Args[1]
		h         = job.Args[2]
		w         = job.Args[3]
		container string
		vmid      string
	)

	if strings.Contains(podID, "pod-") {
		container = ""
		vmid, err = daemon.GetPodVmByName(podID)
		if err != nil {
			return err
		}
	} else if strings.Contains(podID, "vm-") {
		vmid = podID
	} else {
		container = podID
		podID, err = daemon.GetPodByContainer(container)
		if err != nil {
			return err
		}
		vmid, err = daemon.GetPodVmByName(podID)
		if err != nil {
			return err
		}
	}

	row, err := strconv.Atoi(h)
	if err != nil {
		glog.Warning("Success to resize the tty!")
	}
	column, err := strconv.Atoi(w)
	if err != nil {
		glog.Warning("Success to resize the tty!")
	}
	var ttySizeCommand = &hypervisor.WindowSizeCommand{
		ClientTag: tag,
		Size:      &hypervisor.WindowSize{Row: uint16(row), Column: uint16(column)},
	}

	qemuEvent, _, _, err := daemon.GetQemuChan(vmid)
	if err != nil {
		return err
	}
	qemuEvent.(chan hypervisor.VmEvent) <- ttySizeCommand
	glog.V(1).Infof("Success to resize the tty!")
	return nil
}
