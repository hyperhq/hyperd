package hypervisor

import (
	"errors"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/vsock"
)

type BootConfig struct {
	CPU              int
	Memory           int
	BootToBeTemplate bool
	BootFromTemplate bool
	EnableVsock      bool
	EnableVhostUser  bool
	MemoryPath       string
	DevicesStatePath string
	Kernel           string
	Initrd           string
	Bios             string
	Cbfs             string

	// For network QoS (kilobytes/s)
	InboundAverage  string
	InboundPeak     string
	OutboundAverage string
	OutboundPeak    string
}

type HostNicInfo struct {
	Id      string
	Device  string
	Mac     string
	Bridge  string
	Gateway string
	Options string
}

type GuestNicInfo struct {
	Device  string
	Ipaddr  string
	Index   int
	Busaddr int
}

type HypervisorDriver interface {
	Name() string
	InitContext(homeDir string) DriverContext

	LoadContext(persisted map[string]interface{}) (DriverContext, error)

	SupportLazyMode() bool
	SupportVmSocket() bool
}

type BuildinNetworkDriver interface {
	HypervisorDriver

	InitNetwork(bIface, bIP string, disableIptables bool) error
}

var HDriver HypervisorDriver
var VsockCidManager vsock.VsockCidAllocator

type DriverContext interface {
	Launch(ctx *VmContext)
	Associate(ctx *VmContext)
	Dump() (map[string]interface{}, error)

	AddDisk(ctx *VmContext, sourceType string, blockInfo *DiskDescriptor, result chan<- VmEvent)
	RemoveDisk(ctx *VmContext, blockInfo *DiskDescriptor, callback VmEvent, result chan<- VmEvent)

	AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo, result chan<- VmEvent)
	RemoveNic(ctx *VmContext, n *InterfaceCreated, callback VmEvent, result chan<- VmEvent)

	SetCpus(ctx *VmContext, cpus int) error
	AddMem(ctx *VmContext, slot, size int) error

	Save(ctx *VmContext, path string) error

	Shutdown(ctx *VmContext)
	Kill(ctx *VmContext)

	Pause(ctx *VmContext, pause bool) error

	Stats(ctx *VmContext) (*types.PodStats, error)

	Close()
}

type ConsoleDriverContext interface {
	DriverContext

	ConnectConsole(console chan<- string) error
}

type LazyDriverContext interface {
	DriverContext

	LazyLaunch(ctx *VmContext)
	InitVM(ctx *VmContext) error
	LazyAddDisk(ctx *VmContext, name, sourceType, filename, format string, id int)
	LazyAddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo)
}

type EmptyDriver struct{}

type EmptyContext struct{}

func (ed *EmptyDriver) Initialize() error {
	return nil
}

func (ed *EmptyDriver) Name() string {
	return "empty"
}

func (ed *EmptyDriver) InitContext(homeDir string) DriverContext {
	return &EmptyContext{}
}

func (ed *EmptyDriver) LoadContext(persisted map[string]interface{}) (DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "empty" {
		return nil, errors.New("wrong driver type in persist info")
	}
	return &EmptyContext{}, nil
}

func (ed *EmptyDriver) SupportLazyMode() bool {
	return false
}

func (ed *EmptyDriver) SupportVmSocket() bool {
	return false
}

func (ec *EmptyContext) Launch(ctx *VmContext) {}

func (ec *EmptyContext) Associate(ctx *VmContext) {}

func (ec *EmptyContext) Dump() (map[string]interface{}, error) {
	return map[string]interface{}{"hypervisor": "empty"}, nil
}

func (ec *EmptyContext) AddDisk(ctx *VmContext, sourceType string, blockInfo *DiskDescriptor, result chan<- VmEvent) {
}

func (ec *EmptyContext) RemoveDisk(ctx *VmContext, blockInfo *DiskDescriptor, callback VmEvent, result chan<- VmEvent) {
}

func (ec *EmptyContext) AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo, result chan<- VmEvent) {
}

func (ec *EmptyContext) RemoveNic(ctx *VmContext, n *InterfaceCreated, callback VmEvent, result chan<- VmEvent) {
}

func (ec *EmptyContext) SetCpus(ctx *VmContext, cpus int) error      { return nil }
func (ec *EmptyContext) AddMem(ctx *VmContext, slot, size int) error { return nil }

func (ec *EmptyContext) Save(ctx *VmContext, path string) error { return nil }

func (ec *EmptyContext) Shutdown(ctx *VmContext) {}

func (ec *EmptyContext) Kill(ctx *VmContext) {}

func (ec *EmptyContext) Pause(ctx *VmContext, pause bool) error { return nil }

func (ec *EmptyContext) BuildinNetwork() bool { return false }

func (ec *EmptyContext) ConfigureNetwork(config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (ec *EmptyContext) ReleaseNetwork(releasedIP string) error {
	return nil
}

func (ec *EmptyContext) Stats(ctx *VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (ec *EmptyContext) Close() {}
