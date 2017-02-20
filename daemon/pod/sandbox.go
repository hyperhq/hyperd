package pod

import (
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/runv/factory"
	"github.com/hyperhq/runv/hypervisor"
)

const (
	maxReleaseRetry = 3
)

func startSandbox(f factory.Factory, cpu, mem int, kernel, initrd string) (vm *hypervisor.Vm, err error) {
	var (
		DEFAULT_CPU = 1
		DEFAULT_MEM = 128
	)

	if cpu <= 0 {
		cpu = DEFAULT_CPU
	}
	if mem <= 0 {
		mem = DEFAULT_MEM
	}

	if kernel == "" {
		hlog.Log(DEBUG, "get sandbox from factory: CPU: %d, Memory %d", cpu, mem)
		vm, err = f.GetVm(cpu, mem)
	} else {
		hlog.Log(DEBUG, "The create sandbox with: kernel=%s, initrd=%s, cpu=%d, memory=%d", kernel, initrd, cpu, mem)
		config := &hypervisor.BootConfig{
			CPU:    cpu,
			Memory: mem,
			Kernel: kernel,
			Initrd: initrd,
		}
		vm, err = hypervisor.GetVm("", config, false)
	}
	if err != nil {
		hlog.Log(ERROR, "failed to create a sandbox (cpu=%d, mem=%d kernel=%s initrd=%d): %v", cpu, mem, kernel, initrd, err)
	}

	return vm, err
}

func dissociateSandbox(sandbox *hypervisor.Vm, retry int) error {
	if sandbox == nil {
		return nil
	}

	err := sandbox.ReleaseVm()
	if err != nil {
		hlog.Log(WARNING, "SB[%s] failed to release sandbox: %v", sandbox.Id, err)
		hlog.Log(INFO, "SB[%s] shutdown because of failed release", sandbox.Id)
		sandbox.Kill()
		return err
	}
	return nil
}
