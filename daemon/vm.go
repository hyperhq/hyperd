package daemon

import (
	"fmt"
	"strconv"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CmdVmCreate(job *engine.Job) (err error) {
	var (
		vm    *hypervisor.Vm
		cpu   = 1
		mem   = 128
		async = false
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

	if job.Args[2] == "yes" { //async
		async = true
	}

	vm, err = daemon.vmCache.get(cpu, mem)
	if err != nil {
		return err
	}

	cleanup := func() {
		if err != nil {
			daemon.KillVm(vm.Id)
		}
	}

	defer cleanup()

	if !async {
		err = daemon.WaitVmStart(vm)
		if err != nil {
			return err
		}
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
	}

	return ret, err
}

func (daemon *Daemon) StartVm(vmId string, cpu, mem int, lazy, cache bool, keep int) (*hypervisor.Vm, error) {
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
		CPU:          cpu,
		Memory:       mem,
		HotAddCpuMem: cache,
		Kernel:       daemon.Kernel,
		Initrd:       daemon.Initrd,
		Bios:         daemon.Bios,
		Cbfs:         daemon.Cbfs,
		Vbox:         daemon.VboxImage,
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
		return daemon.StartVm("", resource.Vcpu, resource.Memory, lazy, false, keep)
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

type VmCache struct {
	daemon     *Daemon
	lo, hi, nr int
	cached     []*hypervisor.Vm
}

func (c *VmCache) get(cpu, mem int) (*hypervisor.Vm, error) {
	glog.V(2).Info("Try to get cached Vm")
	defer c.fill()
	if c.nr == 0 {
		return c.daemon.StartVm("", cpu, mem, false, false, 0)
	}
	c.nr = c.nr - 1
	vm := c.cached[c.nr]
	c.cached[c.nr] = nil
	glog.V(2).Info("Get cached Vm: %s", vm.Id)

	// hotplug add cpu and memory
	var err error
	if vm.Cpu < cpu {
		glog.Info("HotAddCpu for cached Vm")
		err = vm.AddCpu(cpu)
		glog.Info("HotAddCpu result %v", err)
	}
	if vm.Mem < mem {
		glog.Info("HotAddMem for cached Vm")
		err = vm.AddMem(mem)
		glog.Info("HotAddMem result %v", err)
	}
	if err != nil {
		c.daemon.KillVm(vm.Id)
		vm = nil
	}
	return vm, err
}

func (c *VmCache) fill() {
	for c.nr < c.lo {
		vm, err := c.daemon.StartVm("", 0, 0, false, true, 0)
		if err != nil {
			glog.Info("VmCache fills failed:%v", err)
			return
		}
		c.cached[c.nr] = vm
		c.nr = c.nr + 1
	}
}

func (c *VmCache) put(vm *hypervisor.Vm) {
	if c.nr < c.hi && vm.Cpu == 1 && vm.Mem == 128 { // default VM
		glog.V(2).Info("VmCache return one vm")
		c.cached[c.nr] = vm
		c.nr = c.nr + 1
	} else {
		c.daemon.KillVm(vm.Id)
	}
}

func (daemon *Daemon) InitVmCache(cache_policy string) error {
	daemon.vmCache.daemon = daemon

	if hypervisor.HDriver.SupportLazyMode() {
		cache_policy = "none"
	}

	switch cache_policy {
	case "none", "":
		return nil
	case "cache":
		daemon.vmCache.lo = 10
		daemon.vmCache.hi = 20
		daemon.vmCache.cached = make([]*hypervisor.Vm, daemon.vmCache.hi)
		daemon.vmCache.fill()
		return nil
	case "clone":
		return fmt.Errorf("unimplemented cache policy: clone")
	default:
		return fmt.Errorf("unknown cache policy: %s", cache_policy)
	}
}
