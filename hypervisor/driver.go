package hypervisor

import "errors"

type BootConfig struct {
	CPU    int
	Memory int
	Kernel string
	Initrd string
	Bios   string
	Cbfs   string
}

type HostNicInfo struct {
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
	Initialize() error

	InitContext(homeDir string) DriverContext

	LoadContext(persisted map[string]interface{}) (DriverContext, error)
}

type DriverContext interface {
	Launch(ctx *VmContext)
	Associate(ctx *VmContext)
	Dump() (map[string]interface{}, error)

	AddDisk(ctx *VmContext, name, sourceType, filename, format string, id int)
	RemoveDisk(ctx *VmContext, filename, format string, id int, callback VmEvent)

	AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo)
	RemoveNic(ctx *VmContext, device, mac string, callback VmEvent)

	Shutdown(ctx *VmContext)
	Kill(ctx *VmContext)

	BuildinNetwork() bool
	Close()
}

type EmptyDriver struct{}

type EmptyContext struct{}

func (ed *EmptyDriver) Initialize() error {
	return nil
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

func (ec *EmptyContext) Launch(ctx *VmContext) {}

func (ec *EmptyContext) Associate(ctx *VmContext) {}

func (ec *EmptyContext) Dump() (map[string]interface{}, error) {
	return map[string]interface{}{"hypervisor": "empty"}, nil
}

func (ec *EmptyContext) AddDisk(ctx *VmContext, name, sourceType, filename, format string, id int) {}

func (ec *EmptyContext) RemoveDisk(ctx *VmContext, filename, format string, id int, callback VmEvent) {
}

func (ec *EmptyContext) AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo) {}

func (ec *EmptyContext) RemoveNic(ctx *VmContext, device, mac string, callback VmEvent) {}

func (ec *EmptyContext) Shutdown(ctx *VmContext) {}

func (ec *EmptyContext) Kill(ctx *VmContext) {}

func (ec *EmptyContext) BuildinNetwork() bool { return false }

func (ec *EmptyContext) Close() {}
