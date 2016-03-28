package daemon

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CreateVm(cpu, mem int, async bool) (*hypervisor.Vm, error) {
	vm, err := daemon.StartVm("", cpu, mem, false, 0)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			daemon.KillVm(vm.Id)
		}
	}()

	if !async {
		err = daemon.WaitVmStart(vm)
		if err != nil {
			return nil, err
		}
	}

	return vm, nil
}

func (daemon *Daemon) KillVm(vmId string) (int, string, error) {
	vm, ok := daemon.VmList[vmId]
	if !ok {
		return 0, "", nil
	}
	code, cause, err := vm.Kill()
	if err == nil {
		daemon.RemoveVm(vmId)
	}

	return code, cause, err
}

func (p *Pod) AssociateVm(daemon *Daemon, vmId string) error {
	if p.vm != nil && p.vm.Id != vmId {
		return fmt.Errorf("pod %s already has vm %s, but trying to associate with %s", p.id, p.vm.Id, vmId)
	} else if p.vm != nil {
		return nil
	}

	vmData, err := daemon.GetVmData(vmId)
	if err != nil {
		return err
	}
	glog.V(1).Infof("The data for vm(%s) is %v", vmId, vmData)

	p.vm = daemon.NewVm(vmId, p.spec.Resource.Vcpu, p.spec.Resource.Memory, false, types.VM_KEEP_NONE)
	p.status.Vm = vmId

	err = p.vm.AssociateVm(p.status, vmData)
	if err != nil {
		p.vm = nil
		p.status.Vm = ""
		return err
	}

	daemon.AddVm(p.vm)
	return nil
}

func (daemon *Daemon) ReleaseAllVms() (int, error) {
	var (
		ret       = types.E_OK
		err error = nil
	)

	for _, vm := range daemon.VmList {
		ret, err = vm.ReleaseVm()
		if err != nil {
			/* FIXME: continue to release other vms? */
			break
		}
		daemon.RemoveVm(vm.Id)
	}

	return ret, err
}

func (daemon *Daemon) StartVm(vmId string, cpu, mem int, lazy bool, keep int) (vm *hypervisor.Vm, err error) {
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

	b := &hypervisor.BootConfig{
		CPU:    cpu,
		Memory: mem,
		Kernel: daemon.Kernel,
		Initrd: daemon.Initrd,
		Bios:   daemon.Bios,
		Cbfs:   daemon.Cbfs,
		Vbox:   daemon.VboxImage,
	}

	glog.V(1).Infof("The config: kernel=%s, initrd=%s", daemon.Kernel, daemon.Initrd)
	vm, err = hypervisor.GetVm(vmId, b, false, lazy, keep)
	if err == nil {
		daemon.AddVm(vm)
	}
	return vm, err
}

func (daemon *Daemon) WaitVmStart(vm *hypervisor.Vm) error {
	Status, err := vm.GetResponseChan()
	if err != nil {
		return err
	}
	defer vm.ReleaseResponseChan(Status)

	vmResponse := <-Status
	glog.V(1).Infof("Get the response from VM, VM id is %s, response code is %d!", vmResponse.VmId, vmResponse.Code)
	if vmResponse.Code != types.E_VM_RUNNING {
		return fmt.Errorf("Vbox does not start successfully")
	}
	return nil
}

func (daemon *Daemon) GetVM(vmId string, resource *pod.UserResource, lazy bool, keep int) (*hypervisor.Vm, error) {
	if vmId == "" {
		return daemon.StartVm("", resource.Vcpu, resource.Memory, lazy, keep)
	}

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return nil, fmt.Errorf("The VM %s doesn't exist", vmId)
	}
	/* FIXME: check if any pod is running on this vm? */
	glog.Infof("find vm:%s", vm.Id)
	if resource.Vcpu != vm.Cpu {
		return nil, fmt.Errorf("The new pod's cpu setting is different with the VM's cpu")
	}

	if resource.Memory != vm.Mem {
		return nil, fmt.Errorf("The new pod's memory setting is different with the VM's memory")
	}

	return vm, nil
}

func (daemon *Daemon) NewVm(id string, cpu, memory int, lazy bool, keep int) *hypervisor.Vm {
	vmId := id

	if vmId == "" {
		for {
			vmId = fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
			if _, ok := daemon.VmList[vmId]; !ok {
				break
			}
		}
	}
	return hypervisor.NewVm(vmId, cpu, memory, lazy, keep)
}
