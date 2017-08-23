package hypervisor

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/network"
)

const (
	MAX_NIC = int(^uint(0) >> 1) // Eth is network card, while lo is alias, what's the maximum for each? same?
	// let upper level logic care about the restriction. here is just an upbond.
	DEFAULT_LO_DEVICE_NAME = "lo"
)

type NetworkContext struct {
	*api.SandboxConfig

	sandbox *VmContext

	ports []*api.PortDescription
	eth   map[int]*InterfaceCreated
	lo    map[string]*InterfaceCreated

	idMap map[string]*InterfaceCreated // a secondary index for both eth and lo, for lo, the hostdevice is empty

	slotLock *sync.RWMutex
}

func NewNetworkContext() *NetworkContext {
	return &NetworkContext{
		ports:    []*api.PortDescription{},
		eth:      make(map[int]*InterfaceCreated),
		lo:       make(map[string]*InterfaceCreated),
		idMap:    make(map[string]*InterfaceCreated),
		slotLock: &sync.RWMutex{},
	}
}

func (nc *NetworkContext) sandboxInfo() *hyperstartapi.Pod {

	vmSpec := NewVmSpec()

	vmSpec.Hostname = nc.Hostname
	vmSpec.Dns = nc.Dns
	vmSpec.DnsSearch = nc.DnsSearch
	vmSpec.DnsOptions = nc.DnsOptions
	if nc.Neighbors != nil {
		vmSpec.PortmappingWhiteLists = &hyperstartapi.PortmappingWhiteList{
			InternalNetworks: nc.Neighbors.InternalNetworks,
			ExternalNetworks: nc.Neighbors.ExternalNetworks,
		}
	}

	return vmSpec
}

func (nc *NetworkContext) applySlot() int {
	for i := 0; i <= MAX_NIC; i++ {
		if _, ok := nc.eth[i]; !ok {
			nc.eth[i] = nil
			return i
		}
	}

	return -1
}

func (nc *NetworkContext) freeSlot(slot int) {
	if inf, ok := nc.eth[slot]; !ok {
		nc.sandbox.Log(WARNING, "Freeing an unoccupied eth slot %d", slot)
		return
	} else if inf != nil {
		if _, ok := nc.idMap[inf.Id]; ok {
			delete(nc.idMap, inf.Id)
		}
	}
	nc.sandbox.Log(DEBUG, "Free slot %d of eth", slot)
	delete(nc.eth, slot)
}

// nextAvailableDevName find the initial device name in guest when add a new tap device
// then rename it to some new name.
// For example: user want to insert a new nic named "eth5" into container, which already owns
// "eth0", "eth3" and "eth4". After tap device is added to VM, guest will detect a new device
// named "eth1" which is first available "ethX" device, then guest will try to rename "eth1" to
// "eth5". Then container will have "eth0", "eth3", "eth4" and "eth5" in the last.
// This function try to find the first available "ethX" as said above. @WeiZhang555
func (nc *NetworkContext) nextAvailableDevName() string {
	for i := 0; i <= MAX_NIC; i++ {
		find := false
		for _, inf := range nc.eth {
			if inf != nil && inf.NewName == fmt.Sprintf("eth%d", i) {
				find = true
				break
			}
		}
		if !find {
			return fmt.Sprintf("eth%d", i)
		}
	}

	return ""
}

func (nc *NetworkContext) addInterface(inf *api.InterfaceDescription, result chan api.Result) {
	if inf.Lo {
		if len(inf.Ip) == 0 {
			estr := fmt.Sprintf("creating an interface without an IP address: %#v", inf)
			nc.sandbox.Log(ERROR, estr)
			result <- NewSpecError(inf.Id, estr)
			return
		}
		if inf.Id == "" {
			inf.Id = "lo"
		}
		i := &InterfaceCreated{
			Id:         inf.Id,
			DeviceName: DEFAULT_LO_DEVICE_NAME,
			IpAddr:     inf.Ip,
			Mtu:        inf.Mtu,
		}
		nc.lo[inf.Ip] = i
		nc.idMap[inf.Id] = i

		result <- &api.ResultBase{
			Id:      inf.Id,
			Success: true,
		}
		return
	}

	var devChan chan VmEvent = make(chan VmEvent, 1)

	go func() {
		nc.slotLock.Lock()
		defer nc.slotLock.Unlock()

		idx := nc.applySlot()
		initialDevName := nc.nextAvailableDevName()
		if inf.Id == "" {
			inf.Id = fmt.Sprintf("%d", idx)
		}
		if idx < 0 || initialDevName == "" {
			estr := fmt.Sprintf("no available ethernet slot/name for interface %#v", inf)
			nc.sandbox.Log(ERROR, estr)
			result <- NewBusyError(inf.Id, estr)
			close(devChan)
			return
		}

		nc.configureInterface(idx, nc.sandbox.NextPciAddr(), initialDevName, inf, devChan)
	}()

	go func() {
		ev, ok := <-devChan
		if !ok {
			nc.sandbox.Log(ERROR, "chan closed while waiting network inserted event: %#v", ev)
			return
		}
		// ev might be DeviceInsert failed, or inserted
		if fe, ok := ev.(*DeviceFailed); ok {
			if inf, ok := fe.Session.(*InterfaceCreated); ok {
				nc.netdevInsertFailed(inf.Index, inf.DeviceName)
				nc.sandbox.Log(ERROR, "interface creation failed: %#v", inf)
			} else if inf, ok := fe.Session.(*NetDevInsertedEvent); ok {
				nc.netdevInsertFailed(inf.Index, inf.DeviceName)
				nc.sandbox.Log(ERROR, "interface creation failed: %#v", inf)
			}
			result <- fe
			return
		} else if ni, ok := ev.(*NetDevInsertedEvent); ok {
			created := nc.idMap[inf.Id]
			created.TapFd = ni.TapFd
			nc.sandbox.Log(DEBUG, "nic insert success: %s", ni.Id)
			result <- ni
			return
		}
		nc.sandbox.Log(ERROR, "got unknown event while waiting network inserted event: %#v", ev)
		result <- NewDeviceError(inf.Id, "unknown event")
	}()
}

func (nc *NetworkContext) removeInterface(id string, result chan api.Result) {
	if inf, ok := nc.idMap[id]; !ok {
		nc.sandbox.Log(WARNING, "trying remove a non-exist interface %s", id)
		result <- api.NewResultBase(id, true, "not exist")
		return
	} else if inf.HostDevice == "" { // a virtual interface
		delete(nc.idMap, id)
		delete(nc.lo, inf.IpAddr)
		result <- api.NewResultBase(id, true, "")
		return
	} else {
		nc.slotLock.Lock()
		defer nc.slotLock.Unlock()

		if _, ok := nc.eth[inf.Index]; !ok {
			delete(nc.idMap, id)
			nc.sandbox.Log(INFO, "non-configured network device %d remove failed", inf.Index)
			result <- api.NewResultBase(id, true, "not configured eth")
			return
		}

		var devChan chan VmEvent = make(chan VmEvent, 1)

		nc.sandbox.Log(DEBUG, "remove network card %d: %s", inf.Index, inf.IpAddr)
		nc.sandbox.DCtx.RemoveNic(nc.sandbox, inf, &NetDevRemovedEvent{Index: inf.Index}, devChan)

		go func() {
			ev, ok := <-devChan
			if !ok {
				return
			}

			success := true
			message := ""

			if fe, ok := ev.(*DeviceFailed); ok {
				success = false
				message = "unplug failed"
				if inf, ok := fe.Session.(*NetDevRemovedEvent); ok {
					nc.sandbox.Log(ERROR, "interface remove failed: %#v", inf)
				}
			}

			nc.slotLock.Lock()
			defer nc.slotLock.Unlock()
			nc.freeSlot(inf.Index)
			nc.cleanupInf(inf)

			result <- api.NewResultBase(id, success, message)
		}()
	}
}

// allInterfaces return all the network interfaces except loop
func (nc *NetworkContext) allInterfaces() (nics []*InterfaceCreated) {
	nc.slotLock.Lock()
	defer nc.slotLock.Unlock()

	for _, v := range nc.eth {
		if v != nil {
			nics = append(nics, v)
		}
	}
	return
}

func (nc *NetworkContext) updateInterface(inf *api.InterfaceDescription) error {
	oldInf, ok := nc.idMap[inf.Id]
	if !ok {
		nc.sandbox.Log(WARNING, "trying update a non-exist interface %s", inf.Id)
		return fmt.Errorf("interface %q not exists", inf.Id)
	}

	// only support update some fields: Name, ip addresses, mtu
	nc.slotLock.Lock()
	defer nc.slotLock.Unlock()

	if inf.Name != "" {
		oldInf.NewName = inf.Name
	}

	if inf.Mtu > 0 {
		oldInf.Mtu = inf.Mtu
	}

	if len(inf.Ip) > 0 {
		addrs := strings.Split(inf.Ip, ",")
		oldAddrs := strings.Split(oldInf.IpAddr, ",")
		for _, ip := range addrs {
			var found bool
			if ip[0] == '-' { // to delete
				ip = ip[1:]
				for k, i := range oldAddrs {
					if i == ip {
						oldAddrs = append(oldAddrs[:k], oldAddrs[k+1:]...)
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("failed to delete %q: not found", ip)
				}
			} else { // to add
				oldAddrs = append(oldAddrs, ip)
			}
		}
		oldInf.IpAddr = strings.Join(oldAddrs, ",")
	}
	return nil
}

func (nc *NetworkContext) netdevInsertFailed(idx int, name string) {
	nc.slotLock.Lock()
	defer nc.slotLock.Unlock()

	if _, ok := nc.eth[idx]; !ok {
		nc.sandbox.Log(INFO, "network device %d (%s) insert failed before configured", idx, name)
		return
	}

	nc.sandbox.Log(INFO, "network device %d (%s) insert failed", idx, name)
	nc.freeSlot(idx)
}

func (nc *NetworkContext) configureInterface(index, pciAddr int, name string, inf *api.InterfaceDescription, result chan<- VmEvent) {
	if inf.TapName == "" {
		inf.TapName = network.NicName(nc.sandbox.Id, index)
	}

	settings, err := network.Configure(inf)
	if err != nil {
		nc.sandbox.Log(ERROR, "interface creating failed: %v", err.Error())
		session := &InterfaceCreated{Id: inf.Id, Index: index, PCIAddr: pciAddr, DeviceName: name, NewName: inf.Name, Mtu: inf.Mtu}
		result <- &DeviceFailed{Session: session}
		return
	}

	created, err := interfaceGot(inf.Id, index, pciAddr, name, inf.Name, settings)
	if err != nil {
		result <- &DeviceFailed{Session: created}
		return
	}

	h := &HostNicInfo{
		Id:      created.Id,
		Device:  created.HostDevice,
		Mac:     created.MacAddr,
		Bridge:  created.Bridge,
		Gateway: created.Bridge,
		Options: inf.Options,
	}

	// Note: Use created.NewName add tap name
	// this is because created.DeviceName isn't always uniq,
	// instead NewName is real nic name in VM which is certainly uniq
	g := &GuestNicInfo{
		Device:  created.NewName,
		Ipaddr:  created.IpAddr,
		Index:   created.Index,
		Busaddr: created.PCIAddr,
	}

	nc.eth[index] = created
	nc.idMap[created.Id] = created
	nc.sandbox.DCtx.AddNic(nc.sandbox, h, g, result)
}

func (nc *NetworkContext) cleanupInf(inf *InterfaceCreated) {
	network.ReleaseAddr(inf.IpAddr)
}

func (nc *NetworkContext) getInterface(id string) *InterfaceCreated {
	nc.slotLock.RLock()
	defer nc.slotLock.RUnlock()

	inf, ok := nc.idMap[id]
	if ok {
		return inf
	}
	return nil
}

func (nc *NetworkContext) getIPAddrs() []string {
	nc.slotLock.RLock()
	defer nc.slotLock.RUnlock()

	res := []string{}
	for _, inf := range nc.eth {
		if inf.IpAddr != "" {
			addrs := strings.Split(inf.IpAddr, ",")
			res = append(res, addrs...)
		}
	}

	return res
}

func (nc *NetworkContext) getRoutes() []hyperstartapi.Route {
	nc.slotLock.RLock()
	defer nc.slotLock.RUnlock()
	routes := []hyperstartapi.Route{}

	for _, inf := range nc.idMap {
		for _, r := range inf.RouteTable {
			routes = append(routes, hyperstartapi.Route{
				Dest:    r.Destination,
				Gateway: r.Gateway,
				Device:  inf.NewName,
			})
		}
	}

	return routes
}

func (nc *NetworkContext) close() {
	nc.slotLock.Lock()
	defer nc.slotLock.Unlock()

	for _, inf := range nc.eth {
		nc.cleanupInf(inf)
	}
	nc.eth = map[int]*InterfaceCreated{}
	nc.lo = map[string]*InterfaceCreated{}
	nc.idMap = map[string]*InterfaceCreated{}
}

func interfaceGot(id string, index int, pciAddr int, deviceName, newName string, inf *network.Settings) (*InterfaceCreated, error) {
	rt := []*RouteRule{}
	/* Route rule is generated automaticly on first interface,
	 * or generated on the gateway configured interface. */
	if (index == 0 && inf.Automatic) || (!inf.Automatic && inf.Gateway != "") {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	infc := &InterfaceCreated{
		Id:         id,
		Index:      index,
		PCIAddr:    pciAddr,
		Bridge:     inf.Bridge,
		HostDevice: inf.Device,
		DeviceName: deviceName,
		NewName:    newName,
		MacAddr:    inf.Mac,
		IpAddr:     inf.IPAddress,
		Mtu:        inf.Mtu,
		RouteTable: rt,
	}
	return infc, nil
}
