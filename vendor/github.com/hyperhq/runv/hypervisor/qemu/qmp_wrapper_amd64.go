// +build linux,amd64

package qemu

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, fd int, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", guest.Busaddr)
	commands := []*QmpCommand{}
	if ctx.Boot.EnableVhostUser {
		chardevId := guest.Device + "-chardev"
		commands = append(commands, &QmpCommand{
			Execute: "chardev-add",
			Arguments: map[string]interface{}{
				"id": chardevId,
				"backend": map[string]interface{}{
					"type": "socket",
					"data": map[string]interface{}{
						"addr": map[string]interface{}{
							"type": "unix",
							"data": map[string]interface{}{
								"path": ctx.HomeDir + "/" + host.Id,
							},
						},
						"wait":   false,
						"server": true,
					},
				},
			},
		}, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type":       "vhost-user",
				"id":         guest.Device,
				"chardev":    chardevId,
				"vhostforce": true,
			},
		})
	} else if fd > 0 {
		scm := syscall.UnixRights(fd)
		glog.V(1).Infof("send net to qemu at %d", fd)
		commands = append(commands, &QmpCommand{
			Execute: "getfd",
			Arguments: map[string]interface{}{
				"fdname": "fd" + guest.Device,
			},
			Scm: scm,
		}, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type": "tap", "id": guest.Device, "fd": "fd" + guest.Device,
			},
		})
	} else if host.Device != "" {
		commands = append(commands, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type": "tap", "id": guest.Device, "ifname": host.Device, "script": "no",
			},
		})
	}
	commands = append(commands, &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-pci",
			"netdev": guest.Device,
			"mac":    host.Mac,
			"bus":    "pci.0",
			"addr":   busAddr,
			"id":     guest.Device,
		},
	})

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
