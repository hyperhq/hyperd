// +build linux,amd64

package qemu

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, id, tapname string, fd int, device, mac string, index, addr int, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", addr)
	commands := []*QmpCommand{}
	if ctx.Boot.EnableVhostUser {
		chardevId := device + "-chardev"
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
								"path": ctx.HomeDir + "/" + id,
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
				"id":         device,
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
				"fdname": "fd" + device,
			},
			Scm: scm,
		}, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type": "tap", "id": device, "fd": "fd" + device,
			},
		})
	} else if tapname != "" {
		commands = append(commands, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type": "tap", "id": device, "ifname": tapname, "script": "no",
			},
		})
	}
	commands = append(commands, &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-pci",
			"netdev": device,
			"mac":    mac,
			"bus":    "pci.0",
			"addr":   busAddr,
			"id":     device,
		},
	})

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
