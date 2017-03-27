package direct

import (
	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/hypervisor"
)

type directFactory struct {
	config hypervisor.BootConfig
}

func New(cpu, mem int, kernel, initrd string, vsock bool) base.Factory {
	b := hypervisor.BootConfig{
		CPU:          cpu,
		Memory:       mem,
		HotAddCpuMem: true,
		EnableVsock:  vsock,
		Kernel:       kernel,
		Initrd:       initrd,
	}
	return &directFactory{config: b}
}

func (d *directFactory) Config() *hypervisor.BootConfig {
	config := d.config
	return &config
}

func (d *directFactory) GetBaseVm() (*hypervisor.Vm, error) {
	glog.V(3).Infof("direct factory start create vm")
	vm, err := hypervisor.GetVm("", d.Config(), true)
	if err == nil {
		err = vm.Pause(true)
		if err != nil {
			vm.Kill()
			vm = nil
		}
	}
	if err == nil {
		glog.V(3).Infof("direct factory created vm: %s", vm.Id)
	} else {
		glog.Errorf("direct factory failed to create vm")
	}
	return vm, err
}

func (d *directFactory) CloseFactory() {}
