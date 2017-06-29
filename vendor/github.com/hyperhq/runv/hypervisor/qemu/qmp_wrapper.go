package qemu

import (
	"fmt"
	"strconv"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/utils"
)

func qmpQemuQuit(ctx *hypervisor.VmContext, qc *QemuContext) {
	commands := []*QmpCommand{
		{Execute: "quit", Arguments: map[string]interface{}{}},
	}
	qc.qmp <- &QmpSession{commands: commands, respond: defaultRespond(ctx.Hub, nil)}
}

func scsiId2Name(id int) string {
	return "sd" + utils.DiskId2Name(id)
}

func defaultRespond(result chan<- hypervisor.VmEvent, callback hypervisor.VmEvent) func(err error) {
	return func(err error) {
		if err == nil {
			if callback != nil {
				result <- callback
			}
		} else {
			result <- &hypervisor.DeviceFailed{
				Session: callback,
			}
		}
	}
}

func newDiskDelSession(ctx *hypervisor.VmContext, qc *QemuContext, id int, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	commands := make([]*QmpCommand, 2)
	commands[1] = &QmpCommand{
		Execute: "device_del",
		Arguments: map[string]interface{}{
			"id": "scsi-disk" + strconv.Itoa(id),
		},
	}
	commands[0] = &QmpCommand{
		Execute: "human-monitor-command",
		Arguments: map[string]interface{}{
			"command-line": fmt.Sprintf("drive_del drive%d", id),
		},
	}
	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  defaultRespond(result, callback),
	}
}

func newNetworkDelSession(ctx *hypervisor.VmContext, qc *QemuContext, device string, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	commands := make([]*QmpCommand, 2)
	commands[0] = &QmpCommand{
		Execute: "device_del",
		Arguments: map[string]interface{}{
			"id": device,
		},
	}
	commands[1] = &QmpCommand{
		Execute: "netdev_del",
		Arguments: map[string]interface{}{
			"id": device,
		},
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  defaultRespond(result, callback),
	}
}
