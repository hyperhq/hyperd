package hypervisor

import (
	"fmt"
	"hyper/lib/glog"
	"hyper/network"
	"hyper/pod"
	"net"
	"os"
)

func CreateInterface(index int, pciAddr int, name string, isDefault bool,
	maps []pod.UserContainerPort, callback chan QemuEvent) {
	inf, err := network.Allocate("", maps)
	if err != nil {
		glog.Error("interface creating failed: ", err.Error())
		callback <- &DeviceFailed{
			session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
		}
		return
	}

	interfaceGot(index, pciAddr, name, isDefault, callback, inf)
}

func ReleaseInterface(index int, ipAddr string, file *os.File,
	maps []pod.UserContainerPort, callback chan QemuEvent) {
	success := true
	err := network.Release(ipAddr, maps, file)
	if err != nil {
		glog.Warning("Unable to release network interface, address: ", ipAddr, err)
		success = false
	}
	callback <- &InterfaceReleased{Index: index, Success: success}
}

func interfaceGot(index int, pciAddr int, name string, isDefault bool, callback chan QemuEvent, inf *network.Settings) {

	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		glog.Error("can not parse cidr")
		callback <- &DeviceFailed{
			session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
		}
		return
	}
	var tmp []byte = nw.Mask
	var mask net.IP = tmp

	rt := []*RouteRule{
	//        &RouteRule{
	//            Destination: fmt.Sprintf("%s/%d", nw.IP.String(), inf.IPPrefixLen),
	//            Gateway:"", ViaThis:true,
	//        },
	}
	if isDefault {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	event := &InterfaceCreated{
		Index:      index,
		PCIAddr:    pciAddr,
		DeviceName: name,
		Fd:         inf.File,
		MacAddr:    inf.Mac,
		IpAddr:     ip.String(),
		NetMask:    mask.String(),
		RouteTable: rt,
	}

	callback <- event
}
