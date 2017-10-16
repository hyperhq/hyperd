// +build linux,arm64

package qemu

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, fd int, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", guest.Busaddr)
	commands := make([]*QmpCommand, 3)
	scm := syscall.UnixRights(fd)
	glog.V(1).Infof("send net to qemu at %d", fd)
	commands[0] = &QmpCommand{
		Execute: "getfd",
		Arguments: map[string]interface{}{
			"fdname": "fd" + guest.Device,
		},
		Scm: scm,
	}
	commands[1] = &QmpCommand{
		Execute: "netdev_add",
		Arguments: map[string]interface{}{
			"type": "tap", "id": guest.Device, "fd": "fd" + guest.Device,
		},
	}
	commands[2] = &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"netdev":         guest.Device,
			"driver":         "virtio-net-pci",
			"disable-modern": "off",
			"disable-legacy": "on",
			"bus":            "pci.0",
			"addr":           busAddr,
			"mac":            host.Mac,
			"id":             guest.Device,
		},
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: defaultRespond(result, &hypervisor.NetDevInsertedEvent{
			Id:         host.Id,
			Index:      guest.Index,
			DeviceName: guest.Device,
			Address:    guest.Busaddr,
			TapFd:      fd,
		}),
	}
}
