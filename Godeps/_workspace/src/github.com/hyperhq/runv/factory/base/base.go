package base

import "github.com/hyperhq/runv/hypervisor"

type Factory interface {
	Config() *hypervisor.BootConfig
	// get the base vm of the factory
	// the vm should be stared(init-connected) paused and can be hotadd cpu/mem
	GetBaseVm() (*hypervisor.Vm, error)
	CloseFactory()
}
