package qemu

import (
	"hyper/network"
	"os"
	"strconv"
	"testing"
)

func TestCreateIf(t *testing.T) {
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Error("cannot open /dev/null")
	}

	nw := &network.Settings{
		IPAddress:   "192.168.12.34",
		IPPrefixLen: 25,
		Bridge:      "<nil>",
		Device:      "device",
		Gateway:     "192.168.12.1",
		File:        file,
	}

	res := make(chan QemuEvent, 2)
	interfaceGot(0, 3, "eth0", true, res, nw)

	rsp := <-res
	if rsp.Event() != EVENT_INTERFACE_ADD {
		t.Error("wrong msg type", rsp.Event())
	}

	event := rsp.(*InterfaceCreated)
	t.Log("fd:", event.Fd)

	if event.Index != 0 || event.PCIAddr != 3 || event.DeviceName != "eth0" ||
		event.IpAddr != "192.168.12.34" || event.NetMask != "255.255.255.128" {
		t.Error("info mistake", event.NetMask, event.IpAddr)
	}

	for i := 0; i < len(event.RouteTable); i++ {
		t.Logf("route: %s\t%s\t%s", event.RouteTable[i].Destination, event.RouteTable[i].Gateway,
			strconv.FormatBool(event.RouteTable[i].ViaThis))
	}
	if len(event.RouteTable) != 1 {
		t.Error("route rules:", len(event.RouteTable))
	}
}
