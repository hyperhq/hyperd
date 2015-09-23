package daemon

import (
	"fmt"
	"strconv"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdVmCreate(job *engine.Job) (err error) {
	var (
		vm   *hypervisor.Vm
		cpu  = 1
		mem  = 128
	)

	if job.Args[0] != "" {
		cpu, err = strconv.Atoi(job.Args[0])
		if err != nil {
			return err
		}
	}

	if job.Args[1] != "" {
		mem, err = strconv.Atoi(job.Args[1])
		if err != nil {
			return err
		}
	}

	vm, err = daemon.StartVm("", cpu, mem, false, 0)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			daemon.KillVm(vm.Id)
		}
	}()

	err = daemon.WaitVmStart(vm)
	if err != nil {
		return err
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", vm.Id)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdVmKill(job *engine.Job) error {
	vmId := job.Args[0]
	if _, ok := daemon.VmList[vmId]; !ok {
		return fmt.Errorf("Can not find the VM(%s)", vmId)
	}
	code, cause, err := daemon.KillVm(vmId)
	if err != nil {
		return err
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) KillVm(vmId string) (int, string, error) {
	vm, ok := daemon.VmList[vmId]
	if !ok {
		return 0, "", nil
	}
	ret1, ret2, err := vm.Kill()
	if err == nil {
		daemon.RemoveVm(vmId)
	}
	return ret1, ret2, err
}

// This function will only be invoked during daemon start
func (daemon *Daemon) AssociateAllVms() error {
	for _, mypod := range daemon.PodList {
		if mypod.Vm == "" {
			continue
		}
		podData, err := daemon.GetPodByName(mypod.Id)
		if err != nil {
			continue
		}
		userPod, err := pod.ProcessPodBytes(podData)
		if err != nil {
			continue
		}
		glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)

		vmData, err := daemon.GetVmData(mypod.Vm)
		if err != nil {
			continue
		}
		glog.V(1).Infof("The data for vm(%s) is %v", mypod.Vm, vmData)

		vm := daemon.NewVm(mypod.Vm, userPod.Resource.Vcpu, userPod.Resource.Memory, false, types.VM_KEEP_NONE)

		err = vm.AssociateVm(mypod, vmData)
		if err != nil {
			continue
		}

		daemon.AddVm(vm)
	}
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
	}

	return ret, err
}

func (daemon *Daemon) StartVm(vmId string, cpu, mem int, lazy bool, keep int) (*hypervisor.Vm, error) {
	var (
		DEFAULT_CPU = 1
		DEFAULT_MEM = 128
	)

	if cpu <= 0 { cpu = DEFAULT_CPU }
	if mem <= 0 { mem = DEFAULT_MEM }

	b := &hypervisor.BootConfig{
		CPU:    cpu,
		Memory: mem,
		Kernel: daemon.Kernel,
		Initrd: daemon.Initrd,
		Bios:   daemon.Bios,
		Cbfs:   daemon.Cbfs,
		Vbox:   daemon.VboxImage,
	}

	vm := daemon.NewVm(vmId, cpu, mem, lazy, keep)

	glog.V(1).Infof("The config: kernel=%s, initrd=%s", daemon.Kernel, daemon.Initrd)
	err := vm.Launch(b)
	if err != nil {
		return nil, err
	}

	daemon.AddVm(vm)
	return vm, nil
}

func (daemon *Daemon) WaitVmStart(vm *hypervisor.Vm) error {
	_, r2, _, err1 := vm.GetVmChan()
	if err1 != nil {
		return err1
	}
	vmResponse := <-r2.(chan *types.VmResponse)
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

	return vm,nil
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
