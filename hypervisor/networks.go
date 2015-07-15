package hypervisor

import (
	"fmt"
	"github.com/hyperhq/hyper/lib/glog"
	"github.com/hyperhq/hyper/network"
	"github.com/hyperhq/hyper/pod"
	"net"
	"os"
)

func CreateInterface(index int, pciAddr int, name string, isDefault bool, addrOnly bool,
	maps []pod.UserContainerPort, callback chan VmEvent) {
	inf, err := network.Allocate("", addrOnly, maps)
	if err != nil {
		glog.Error("interface creating failed: ", err.Error())
		callback <- &DeviceFailed{
			Session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
		}
		return
	}

	interfaceGot(index, pciAddr, name, isDefault, callback, inf)
}

func ReleaseInterface(index int, ipAddr string, file *os.File,
	maps []pod.UserContainerPort, callback chan VmEvent) {
	success := true
	err := network.Release(ipAddr, maps, file)
	if err != nil {
		glog.Warning("Unable to release network interface, address: ", ipAddr, err)
		success = false
	}
	callback <- &InterfaceReleased{Index: index, Success: success}
}

func interfaceGot(index int, pciAddr int, name string, isDefault bool, callback chan VmEvent, inf *network.Settings) {

	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		glog.Error("can not parse cidr")
		callback <- &DeviceFailed{
			Session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
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
		Bridge:     inf.Bridge,
		HostDevice: inf.Device,
		DeviceName: name,
		Fd:         inf.File,
		MacAddr:    inf.Mac,
		IpAddr:     ip.String(),
		NetMask:    mask.String(),
		RouteTable: rt,
	}

	callback <- event
}
