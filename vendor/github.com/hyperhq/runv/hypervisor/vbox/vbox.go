package vbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/govbox"
	"github.com/hyperhq/runv/lib/utils"
)

//implement the hypervisor.HypervisorDriver interface
type VBoxDriver struct {
	Machines map[string]*hypervisor.VmContext
}

type VBoxContext struct {
	Driver    *VBoxDriver
	Machine   *virtualbox.Machine
	mediums   []*virtualbox.StorageMedium
	callbacks []hypervisor.VmEvent
}

func vboxContext(ctx *hypervisor.VmContext) *VBoxContext {
	return ctx.DCtx.(*VBoxContext)
}

func InitDriver() *VBoxDriver {
	_, err := exec.LookPath("vboxmanage")
	if err != nil {
		return nil
	}

	vd := &VBoxDriver{}
	vd.Machines = make(map[string]*hypervisor.VmContext)
	return vd
}

func (vd *VBoxDriver) Name() string {
	return "vbox"
}

func (vd *VBoxDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &VBoxContext{
		Driver:  vd,
		Machine: nil,
	}
}

func (vd *VBoxDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "vbox" {
		return nil, fmt.Errorf("wrong driver type in persist info")
	}

	var err error
	var m *virtualbox.Machine

	name, ok := persisted["name"]
	if !ok {
		return nil, fmt.Errorf("cannot read the machine name from persist info")
	} else {
		// Get the Machine object based on the machine name
		m, err = virtualbox.GetMachine(name.(string))
		if err != nil {
			return nil, fmt.Errorf("cannot find the machine based on name(%s)", name)
		}
	}

	return &VBoxContext{
		Driver:  vd,
		Machine: m,
	}, nil
}

// Create VM and start it
func (vc *VBoxContext) Launch(ctx *hypervisor.VmContext) {
	// 0. Find an exist vm
	var exist bool = false
	var m *virtualbox.Machine
	var err error
	m, err = virtualbox.GetMachine(ctx.Id)
	if err == nil {
		exist = true
	}
	// 1. Create and Register a VM
	if exist == false {
		vmRootPath := filepath.Join(hypervisor.BaseDir, "vm")
		os.MkdirAll(vmRootPath, 0755)
		err = vc.CreateVm(ctx.Id, vmRootPath, ctx.HyperSockName, ctx.TtySockName)
		if err != nil {
			glog.Errorf(err.Error())
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + ctx.Id + "), since " + err.Error()}
			return
		}
		m = vc.Machine

		// 2. Limit the CPU and Memory
		m.CPUs = uint(ctx.Boot.CPU)
		m.Memory = uint(ctx.Boot.Memory)
		bootorders := []string{}
		bootorders = append(bootorders, "dvd")
		m.BootOrder = bootorders
		m.OSType = "Linux_64"
		m.Flag = m.Flag | virtualbox.F_longmode | virtualbox.F_vtxux | virtualbox.F_hwvirtex | virtualbox.F_vtxvpid | virtualbox.F_acpi | virtualbox.F_ioapic
		m.Modify([]string{})

		nic := virtualbox.NIC{
			Network:  virtualbox.NICNetNAT,
			Hardware: virtualbox.IntelPro1000MTServer,
			NatNet:   network.BridgeIP,
		}
		for i := 1; i <= ctx.InterfaceCount; i++ {
			err = vc.Machine.SetNIC(i, nic)
			if err != nil {
				glog.Errorf(err.Error())
				ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + ctx.Id + "), since " + err.Error()}
				return
			}
		}
		boot := virtualbox.StorageController{
			SysBus:      virtualbox.SysBusSATA,
			Bootable:    true,
			HostIOCache: true,
			Ports:       5,
		}
		medium := virtualbox.StorageMedium{
			Port:      0,
			Device:    0,
			DriveType: virtualbox.DriveDVD,
			Medium:    ctx.Boot.Vbox,
		}
		if err := m.AddStorageCtl(m.Name, boot); err != nil {
			glog.Errorf(err.Error())
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + m.Name + ") with boot device, since " + err.Error()}
			return
		}
		if err := m.AttachStorage(m.Name, medium); err != nil {
			glog.Errorf(err.Error())
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + m.Name + ") with boot device, since " + err.Error()}
			return
		}

		// 3.5. Create shared dir between host and guest
		if err := vc.AddDir(hypervisor.ShareDirTag, ctx.ShareDir, false); err != nil {
			glog.Errorf(err.Error())
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to start a VM(" + m.Name + "), since " + err.Error()}
			return
		}
	}
	vc.Machine = m
	// 4. Start a VM
	if err := m.Start(); err != nil {
		glog.Errorf(err.Error())
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to start a VM(" + m.Name + "), since " + err.Error()}
		return
	}
	m.State = "running"
	vc.Driver.Machines[vc.Machine.Name] = ctx
}

func (vc *VBoxContext) Associate(ctx *hypervisor.VmContext) {
	vc.Driver.Machines[vc.Machine.Name] = ctx
}

func (vc *VBoxContext) Dump() (map[string]interface{}, error) {
	if vc.Machine == nil {
		return nil, fmt.Errorf("can not serialize VBox context: no process running")
	}

	return map[string]interface{}{
		"hypervisor": "vbox",
		"name":       vc.Machine.Name,
	}, nil
}

func (vc *VBoxContext) Pause(ctx *hypervisor.VmContext, pause bool, result chan<- error) {
	err := fmt.Errorf("doesn't support pause for vbox right now")
	glog.Warning(err)
	result <- err
}

func (vc *VBoxContext) Shutdown(ctx *hypervisor.VmContext) {
	go func() {
		// detach the bootable Disk
		m := vc.Machine
		if m == nil {
			return
		}
		name := m.Name
		m.Poweroff()
		time.Sleep(1 * time.Second)
		if err := vc.detachDisk(name, 0); err != nil {
			glog.Warningf("failed to detach the disk of VBox(%s), %s", name, err.Error())
		}
		if err := m.Delete(); err != nil {
			glog.Warningf("failed to delete the VBox(%s), %s", name, err.Error())
		}
		os.RemoveAll(filepath.Join(hypervisor.BaseDir, "vm", name))

		delete(vc.Driver.Machines, name)
		ctx.Hub <- &hypervisor.VmExit{}
	}()
}

func (vc *VBoxContext) Kill(ctx *hypervisor.VmContext) {
	go func() {
		// detach the bootable Disk
		m := vc.Machine
		if m == nil {
			return
		}
		name := m.Name
		m.Poweroff()
		if err := vc.detachDisk(m.Name, 0); err != nil {
			glog.Warningf("failed to detach the disk of VBox(%s), %s", name, err.Error())
		}
		if err := m.Delete(); err != nil {
			glog.Warningf("failed to delete the VBox(%s), %s", name, err.Error())
		}
		delete(vc.Driver.Machines, name)
		args := fmt.Sprintf("ps aux | grep %s | grep -v grep | awk '{print \"kill -9 \" $2}' | sh", name)
		cmd := exec.Command("/bin/sh", "-c", args)
		if err := cmd.Run(); err != nil {
			ctx.Hub <- &hypervisor.VmKilledEvent{Success: false}
			return
		}
		os.RemoveAll(filepath.Join(hypervisor.BaseDir, "vm", name))

		ctx.Hub <- &hypervisor.VmKilledEvent{Success: true}
	}()
}

func (vc *VBoxContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (vc *VBoxContext) Close() {}

func (vc *VBoxContext) AddDisk(ctx *hypervisor.VmContext, sourceType string, blockInfo *hypervisor.DiskDescriptor, result chan<- hypervisor.VmEvent) {
	name := blockInfo.Name
	filename := blockInfo.Filename
	id := blockInfo.ScsiId

	//	go func() {
	/*
		if sourceType != "vdi" {
			glog.Infof("Disk %s (%s) add failed, unsupported source type", name, filename)
			result <- &hypervisor.DeviceFailed{
				Session: nil,
			}
		}
	*/
	m := vc.Machine
	if m == nil {
		result <- &hypervisor.DeviceFailed{
			Session: nil,
		}
		return
	}
	medium := virtualbox.StorageMedium{
		Port:      uint(id) + 1,
		Device:    0,
		DriveType: virtualbox.DriveHDD,
		Medium:    filename,
		SSD:       false,
	}
	if err := m.AttachStorage(m.Name, medium); err != nil {
		glog.Errorf(err.Error())
		result <- &hypervisor.DeviceFailed{
			Session: nil,
		}
		return
	}
	devName := scsiId2Name(id)
	callback := &hypervisor.BlockdevInsertedEvent{
		Name:       name,
		SourceType: sourceType,
		DeviceName: devName,
		ScsiId:     id,
	}

	glog.V(1).Infof("Disk %s (%s) add succeeded", name, filename)
	result <- callback
	return
	//	}()
}

func (vc *VBoxContext) RemoveDisk(ctx *hypervisor.VmContext, blockInfo *hypervisor.DiskDescriptor, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	filename := blockInfo.Filename
	id := blockInfo.ScsiId

	//	go func() {
	m := vc.Machine
	if m == nil {
		return
	}
	if err := vc.detachDisk(m.Name, id); err != nil {
		glog.Warningf("failed to detach the disk of VBox(%s), %s", m.Name, err.Error())
		/*
			result <- &hypervisor.DeviceFailed{
				Session: callback,
			}
		*/
	}

	glog.V(1).Infof("Disk %s remove succeeded", filename)
	result <- callback
	return
	//	}()
}

// For shared directory between host and guest OS
func (vc *VBoxContext) AddDir(name, path string, readonly bool) error {
	sFolder := virtualbox.SharedFolder{
		Name:      name,
		Path:      path,
		Automount: false,
		Transient: false,
		Readonly:  readonly,
	}
	if err := vc.Machine.AddSharedFolder(vc.Machine.Name, sFolder); err != nil {
		glog.Warningf("The shared folder is failed to add, since %s", err.Error())
		return err
	}
	return nil
}

func (vc *VBoxContext) RemoveDir(name string) error {
	if err := vc.Machine.RemoveSharedFolder(vc.Machine.Name, name); err != nil {
		glog.Warningf("The shared folder is failed to remove, since %s", err.Error())
		return err
	}
	return nil
}

func (vc *VBoxContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	go func() {
		callback := &hypervisor.NetDevInsertedEvent{
			Id:         host.Id,
			Index:      guest.Index,
			DeviceName: guest.Device,
			Address:    guest.Busaddr,
		}
		if guest.Index > 7 || guest.Index < 0 {
			glog.Errorf("Hot adding NIC failed, can not add more than 8 NICs")
			result <- &hypervisor.DeviceFailed{
				Session: callback,
			}
		}
		/*
			if err := vc.Machine.ModifyNIC(guest.Index+1, virtualbox.NICNetNAT, ""); err != nil {
				glog.Errorf("Hot adding NIC failed, %s", err.Error())
				ctx.Hub <- &hypervisor.DeviceFailed{
					Session: callback,
				}
				return
			}
		*/
		glog.V(1).Infof("nic %s insert succeeded", guest.Device)
		result <- callback
		return
	}()
}

func (vc *VBoxContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	go func() {
		/*
			args := "vboxmanage controlvm " + vc.Machine.Name + " nic1 null"
			if err := exec.Command("/bin/sh", "-c", args).Run(); err != nil {
				glog.Errorf(err.Error())
				ctx.Hub <- &hypervisor.DeviceFailed{
					Session: callback,
				}
				return
			}
		*/
		glog.V(1).Infof("nic %s remove succeeded", n.DeviceName)
		result <- callback
		return
	}()
}

func (vc *VBoxContext) SetCpus(ctx *hypervisor.VmContext, cpus int, result chan<- error) {
	result <- fmt.Errorf("SetCpus is unsupported on virtualbox driver")
}

func (vc *VBoxContext) AddMem(ctx *hypervisor.VmContext, slot, size int, result chan<- error) {
	result <- fmt.Errorf("AddMem is unsupported on virtualbox driver")
}

func (vc *VBoxContext) Save(ctx *hypervisor.VmContext, path string, result chan<- error) {
	result <- fmt.Errorf("Save is unsupported on virtualbox driver")
}

// Prepare the conditions for the vm startup
// * Create VM machine
// * Create serial port
func (vc *VBoxContext) CreateVm(name, baseDir, hyperSockFile, ttySockFile string) error {
	machine, err := virtualbox.CreateMachine(name, baseDir)
	if err != nil {
		glog.Errorf(name + " " + err.Error())
		return err
	}
	vc.Machine = machine
	// Call library to assign the serial ports
	// Base on http://en.wikipedia.org/wiki/Input/output_base_address
	// Serial Port 1 is between 0x03F8 to 0x03FF
	// Serial Port 2 is between 0x02F8 to 0x02FF
	err = machine.CreateSerialPort(hyperSockFile, "1", "0x03F8", "4", virtualbox.HOST_MODE_PIPE, true)
	if err != nil {
		glog.Errorf(name + " " + err.Error())
		return err
	}
	err = machine.CreateSerialPort(ttySockFile, "2", "0x02F8", "3", virtualbox.HOST_MODE_PIPE, true)
	if err != nil {
		glog.Errorf(name + " " + err.Error())
		return err
	}
	return nil
}

func (vc *VBoxContext) detachDisk(disk string, port int) error {
	m := vc.Machine
	medium := virtualbox.StorageMedium{
		Port:      uint(port) + 1,
		Device:    0,
		DriveType: virtualbox.DriveHDD,
		Medium:    "none",
	}
	if err := m.AttachStorage(disk, medium); err != nil {
		return err
	}
	return nil
}

func (vc *VBoxDriver) SupportLazyMode() bool {
	return true
}

func scsiId2Name(id int) string {
	return "sd" + utils.DiskId2Name(id)
}

func (vc *VBoxContext) LazyAddDisk(ctx *hypervisor.VmContext, name, sourceType, filename, format string, id int) {
	medium := &virtualbox.StorageMedium{
		Port:      uint(id) + 1,
		Device:    0,
		DriveType: virtualbox.DriveHDD,
		Medium:    filename,
		SSD:       true,
	}
	devName := scsiId2Name(id)
	callback := &hypervisor.BlockdevInsertedEvent{
		Name:       name,
		SourceType: sourceType,
		DeviceName: devName,
		ScsiId:     id,
	}
	vc.mediums = append(vc.mediums, medium)
	vc.callbacks = append(vc.callbacks, callback)
}

func (vc *VBoxContext) LazyAddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
	callback := &hypervisor.NetDevInsertedEvent{
		Id:         host.Id,
		Index:      guest.Index,
		DeviceName: guest.Device,
		Address:    guest.Busaddr,
	}
	vc.callbacks = append(vc.callbacks, callback)
}

func (vc *VBoxContext) InitVM(ctx *hypervisor.VmContext) error {
	vmRootPath := filepath.Join(hypervisor.BaseDir, "vm")
	os.MkdirAll(vmRootPath, 0755)
	m, err := virtualbox.GetMachine(ctx.Id)
	if err != nil {
		if err == virtualbox.ErrMachineNotExist {
			m, err = virtualbox.CreateMachine(ctx.Id, vmRootPath)
		}
		if err != nil {
			glog.Errorf(ctx.Id + " " + err.Error())
			return err
		}
	}
	vc.Machine = m

	modifies := []string{}

	com1 := m.SerialPortConf(ctx.HyperSockName, "1", "0x03F8", "4", virtualbox.HOST_MODE_PIPE, true)
	if len(com1) > 0 {
		modifies = append(modifies, com1...)
	}
	com2 := m.SerialPortConf(ctx.TtySockName, "2", "0x02F8", "3", virtualbox.HOST_MODE_PIPE, true)
	if len(com2) > 0 {
		modifies = append(modifies, com2...)
	}

	// 2. Limit the CPU and Memory
	m.CPUs = uint(ctx.Boot.CPU)
	m.Memory = uint(ctx.Boot.Memory)
	bootorders := []string{}
	bootorders = append(bootorders, "dvd")
	m.BootOrder = bootorders
	m.OSType = "Linux_64"
	m.Flag = m.Flag | virtualbox.F_longmode | virtualbox.F_vtxux | virtualbox.F_hwvirtex | virtualbox.F_vtxvpid | virtualbox.F_acpi | virtualbox.F_ioapic

	nic := virtualbox.NIC{
		Network:  virtualbox.NICNetNAT,
		Hardware: virtualbox.IntelPro1000MTServer,
		NatNet:   network.BridgeIP,
	}
	for i := 1; i <= ctx.InterfaceCount; i++ {
		modifies = append(modifies, m.NicConf(i, nic)...)
	}

	return m.Modify(modifies)
}

func (vc *VBoxContext) LazyLaunch(ctx *hypervisor.VmContext) {

	var err error = nil

	defer func() {
		if err != nil {
			glog.Errorf("fail to start %s, should I delete it?", ctx.Id)
		}
	}()

	m := vc.Machine

	boot := virtualbox.StorageController{
		SysBus:      virtualbox.SysBusSATA,
		Bootable:    true,
		HostIOCache: true,
		Ports:       uint(len(vc.mediums) + 1),
	}
	medium := virtualbox.StorageMedium{
		Port:      0,
		Device:    0,
		DriveType: virtualbox.DriveDVD,
		Medium:    ctx.Boot.Vbox,
		SSD:       false,
	}
	if err = m.AddStorageCtl(m.Name, boot); err != nil {
		glog.Errorf(err.Error())
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + m.Name + ") with boot device, since " + err.Error()}
		return
	}
	if err = m.AttachStorage(m.Name, medium); err != nil {
		glog.Errorf(err.Error())
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + m.Name + ") with boot device, since " + err.Error()}
		return
	}
	for _, disk := range vc.mediums {
		if err = m.AttachStorage(m.Name, *disk); err != nil {
			glog.Errorf(err.Error())
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to create a VM(" + m.Name + ") with boot device, since " + err.Error()}
			return
		}
	}

	// 3.5. Create shared dir between host and guest
	if err := vc.AddDir(hypervisor.ShareDirTag, ctx.ShareDir, false); err != nil {
		glog.Errorf(err.Error())
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to start a VM(" + m.Name + "), since " + err.Error()}
		return
	}

	// 4. Start a VM
	if err := vc.Machine.Start(); err != nil {
		glog.Errorf(err.Error())
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "failed to start a VM(" + m.Name + "), since " + err.Error()}
		return
	}
	vc.Machine.State = "running"
	vc.Driver.Machines[vc.Machine.Name] = ctx

	for _, cb := range vc.callbacks {
		ctx.Hub <- cb
	}
}
