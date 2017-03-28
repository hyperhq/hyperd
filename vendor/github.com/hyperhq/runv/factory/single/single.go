package single

import (
	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/hypervisor"
)

type Factory struct{ base.Factory }

func New(b base.Factory) Factory {
	return Factory{Factory: b}
}

func (f Factory) GetVm(cpu, mem int) (*hypervisor.Vm, error) {
	// check if match the base
	config := f.Config()
	if config.CPU > cpu || config.Memory > mem {
		// also strip unrelated option from @config
		boot := &hypervisor.BootConfig{
			CPU:         cpu,
			Memory:      mem,
			Kernel:      config.Kernel,
			Initrd:      config.Initrd,
			EnableVsock: config.EnableVsock,
		}
		return hypervisor.GetVm("", boot, false)
	}

	vm, err := f.GetBaseVm()
	if err != nil {
		return nil, err
	}

	// unpause
	vm.Pause(false)

	// hotplug add cpu and memory
	var needOnline bool = false
	if vm.Cpu < cpu {
		needOnline = true
		glog.V(3).Info("HotAddCpu for cached Vm")
		err = vm.SetCpus(cpu)
		glog.V(3).Infof("HotAddCpu result %v", err)
	}
	if vm.Mem < mem {
		needOnline = true
		glog.V(3).Info("HotAddMem for cached Vm")
		err = vm.AddMem(mem)
		glog.V(3).Infof("HotAddMem result %v", err)
	}
	if needOnline {
		glog.V(3).Info("OnlineCpuMem for cached Vm")
		vm.OnlineCpuMem()
	}
	if err != nil {
		vm.Kill()
		vm = nil
	}
	return vm, err
}
