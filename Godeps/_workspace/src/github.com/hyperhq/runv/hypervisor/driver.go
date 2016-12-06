package hypervisor

import (
	"errors"
	"os"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
)

type BootConfig struct {
	CPU              int
	Memory           int
	HotAddCpuMem     bool
	BootToBeTemplate bool
	BootFromTemplate bool
	MemoryPath       string
	DevicesStatePath string
	Kernel           string
	Initrd           string
	Bios             string
	Cbfs             string
	Vbox             string

	// For network QoS (kilobytes/s)
	InboundAverage  string
	InboundPeak     string
	OutboundAverage string
	OutboundPeak    string
}

type HostNicInfo struct {
	Id      string
	Fd      uint64
	Device  string
	Mac     string
	Bridge  string
	Gateway string
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

	BuildinNetwork() bool

	InitNetwork(bIface, bIP string, disableIptables bool) error

	SupportLazyMode() bool
}

var HDriver HypervisorDriver

type DriverContext interface {
	Launch(ctx *VmContext)
	Associate(ctx *VmContext)
	Dump() (map[string]interface{}, error)

	AddDisk(ctx *VmContext, sourceType string, blockInfo *DiskDescriptor, result chan<- VmEvent)
	RemoveDisk(ctx *VmContext, blockInfo *DiskDescriptor, callback VmEvent, result chan<- VmEvent)

	AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo, result chan<- VmEvent)
	RemoveNic(ctx *VmContext, n *InterfaceCreated, callback VmEvent, result chan<- VmEvent)

	SetCpus(ctx *VmContext, cpus int, result chan<- error)
	AddMem(ctx *VmContext, slot, size int, result chan<- error)

	Save(ctx *VmContext, path string, result chan<- error)

	Shutdown(ctx *VmContext)
	Kill(ctx *VmContext)

	Pause(ctx *VmContext, pause bool, result chan<- error)

	ConfigureNetwork(vmId, requestedIP string, config *api.InterfaceDescription) (*network.Settings, error)
	AllocateNetwork(vmId, requestedIP string) (*network.Settings, error)
	ReleaseNetwork(vmId, releasedIP string, file *os.File) error

	Stats(ctx *VmContext) (*types.PodStats, error)

	Close()
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

func (ec *EmptyContext) SetCpus(ctx *VmContext, cpus int, result chan<- error) {}
func (ec *EmptyContext) AddMem(ctx *VmContext, slot, size int, result chan<- error) {
}

func (ec *EmptyContext) Save(ctx *VmContext, path string, result chan<- error) {}

func (ec *EmptyContext) Shutdown(ctx *VmContext) {}

func (ec *EmptyContext) Kill(ctx *VmContext) {}

func (ec *EmptyContext) Pause(ctx *VmContext, pause bool, result chan<- error) {}

func (ec *EmptyContext) BuildinNetwork() bool { return false }

func (ec *EmptyContext) ConfigureNetwork(vmId, requestedIP string, config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (ec *EmptyContext) AllocateNetwork(vmId, requestedIP string) (*network.Settings, error) {
	return nil, nil
}

func (ec *EmptyContext) ReleaseNetwork(vmId, releasedIP string, file *os.File) error {
	return nil
}

func (ec *EmptyContext) Stats(ctx *VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (ec *EmptyContext) Close() {}
