// +build linux,amd64

package qemu

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, id string, fd uint64, device, mac string, index, addr int, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", addr)
	commands := make([]*QmpCommand, 3)
	scm := syscall.UnixRights(int(fd))
	glog.V(1).Infof("send net to qemu at %d", int(fd))
	commands[0] = &QmpCommand{
		Execute: "getfd",
		Arguments: map[string]interface{}{
			"fdname": "fd" + device,
		},
		Scm: scm,
	}
	commands[1] = &QmpCommand{
		Execute: "netdev_add",
		Arguments: map[string]interface{}{
			"type": "tap", "id": device, "fd": "fd" + device,
		},
	}
	commands[2] = &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-pci",
			"netdev": device,
			"mac":    mac,
			"bus":    "pci.0",
			"addr":   busAddr,
			"id":     device,
		},
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: defaultRespond(result, &hypervisor.NetDevInsertedEvent{
			Id:         id,
			Index:      index,
			DeviceName: device,
			Address:    addr,
		}),
	}
}
