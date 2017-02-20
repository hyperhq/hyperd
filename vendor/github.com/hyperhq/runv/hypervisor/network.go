package hypervisor

import (
	"fmt"
	"net"
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

func (nc *NetworkContext) addInterface(inf *api.InterfaceDescription, result chan api.Result) {
	if inf.Lo {
		if inf.Ip == "" {
			estr := fmt.Sprintf("creating an interface without an IP address: %#v", inf)
			nc.sandbox.Log(ERROR, estr)
			result <- NewSpecError(inf.Id, estr)
			return
		}
		i := &InterfaceCreated{
			Id:         inf.Id,
			DeviceName: DEFAULT_LO_DEVICE_NAME,
			IpAddr:     inf.Ip,
			NetMask:    "255.255.255.255",
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
		if idx < 0 {
			estr := fmt.Sprintf("no available ethernet slot for interface %#v", inf)
			nc.sandbox.Log(ERROR, estr)
			result <- NewBusyError(inf.Id, estr)
			close(devChan)
			return
		}

		nc.configureInterface(idx, nc.sandbox.NextPciAddr(), fmt.Sprintf("eth%d", idx), inf, devChan)
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
	var (
		err      error
		settings *network.Settings
	)

	if HDriver.BuildinNetwork() {
		/* VBox doesn't support join to bridge */
		settings, err = nc.sandbox.DCtx.ConfigureNetwork(nc.sandbox.Id, "", inf)
	} else {
		settings, err = network.Configure(nc.sandbox.Id, "", false, inf)
	}

	if err != nil {
		nc.sandbox.Log(ERROR, "interface creating failed: %v", err.Error())
		session := &InterfaceCreated{Id: inf.Id, Index: index, PCIAddr: pciAddr, DeviceName: name}
		result <- &DeviceFailed{Session: session}
		return
	}

	created, err := interfaceGot(inf.Id, index, pciAddr, name, settings)
	if err != nil {
		result <- &DeviceFailed{Session: created}
		return
	}

	h := &HostNicInfo{
		Id:      created.Id,
		Fd:      uint64(created.Fd.Fd()),
		Device:  created.HostDevice,
		Mac:     created.MacAddr,
		Bridge:  created.Bridge,
		Gateway: created.Bridge,
	}
	g := &GuestNicInfo{
		Device:  created.DeviceName,
		Ipaddr:  created.IpAddr,
		Index:   created.Index,
		Busaddr: created.PCIAddr,
	}

	nc.eth[index] = created
	nc.idMap[created.Id] = created
	nc.sandbox.DCtx.AddNic(nc.sandbox, h, g, result)
}

func (nc *NetworkContext) cleanupInf(inf *InterfaceCreated) {
	if !HDriver.BuildinNetwork() && inf.Fd != nil {
		network.Close(inf.Fd)
		inf.Fd = nil
	}

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

func (nc *NetworkContext) getIpAddrs() []string {
	nc.slotLock.RLock()
	defer nc.slotLock.RUnlock()

	res := []string{}
	for _, inf := range nc.eth {
		res = append(res, inf.IpAddr)
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
				Device:  inf.DeviceName,
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

func interfaceGot(id string, index int, pciAddr int, name string, inf *network.Settings) (*InterfaceCreated, error) {
	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		return &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}, err
	}
	var tmp []byte = nw.Mask
	var mask net.IP = tmp

	rt := []*RouteRule{}
	/* Route rule is generated automaticly on first interface,
	 * or generated on the gateway configured interface. */
	if (index == 0 && inf.Automatic) || (!inf.Automatic && inf.Gateway != "") {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	return &InterfaceCreated{
		Id:         id,
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
	}, nil
}
