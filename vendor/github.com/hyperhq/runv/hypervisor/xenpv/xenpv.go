// +build linux,with_xen

package xenpv

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/golang/glog"
	api "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	xl "github.com/hyperhq/runv/lib/runvxenlight"
	"github.com/hyperhq/runv/lib/utils"
)

const (
	XENLIGHT_EXEC = "xl"
)

//implement the hypervisor.HypervisorDriver interface
type XenPvDriver struct {
	executable string
	ctx        *xl.Context
}

//implement the hypervisor.DriverContext interface
type XenPvContext struct {
	driver *XenPvDriver
	domId  xl.Domid
}

func InitDriver() *XenPvDriver {
	cmd, err := exec.LookPath(XENLIGHT_EXEC)
	if err != nil {
		return nil
	}

	ctx := &xl.Context{}
	ctx.Open()

	ctx.SigChildHandle()

	return &XenPvDriver{
		executable: cmd,
		ctx:        ctx,
	}
}

func (xd *XenPvDriver) Name() string {
	return "xenpv"
}

func (xd *XenPvDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &XenPvContext{
		driver: xd,
	}
}

func (xd *XenPvDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != xd.Name() {
		return nil, fmt.Errorf("wrong driver type %v in persist info, expect %v", t, xd.Name())
	}

	name, ok := persisted["name"]
	if !ok {
		return nil, fmt.Errorf("there is no xenpv domain name")
	}

	id, err := xd.ctx.DomainQualifierToId(name.(string))
	if err != nil {
		return nil, fmt.Errorf("cannot find domain whose name is %v", name)
	}

	return &XenPvContext{
		driver: xd,
		domId:  id,
	}, nil
}

func (xd *XenPvDriver) SupportLazyMode() bool {
	return false
}

func (xd *XenPvDriver) SupportVmSocket() bool {
	return false
}

func (xc *XenPvContext) Launch(ctx *hypervisor.VmContext) {
	if xc.driver.executable == "" {
		glog.Errorf("can not find xl executable")
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "can not find xl executable"}
		return
	}
	uuid, err := xl.GenerateUuid()
	if err != nil {
		glog.Errorf("generate uuid failed: %v\n", err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}

	boot := ctx.Boot
	config := &xl.DomainConfig{
		Cinfo: xl.CreateInfo{
			Type: "pv",
			Name: ctx.Id,
			Uuid: uuid,
		},
		Binfo: xl.BuildInfo{
			Kernel:            boot.Kernel,
			Initrd:            boot.Initrd,
			Cmdline:           "console=hvc0 " + api.HYPER_P9_USE_XEN,
			ClaimMode:         "True",
			MaxVcpus:          boot.CPU,
			MaxMemory:         uint64(boot.Memory * 1024),
			TargetMemory:      uint64(boot.Memory * 1024),
			DeviceModeVersion: "qemu_xen",
			PvInfo:            xl.PvInfo{},
		},
		Channels: []xl.ChannelInfo{
			{
				DevId:  0,
				Name:   "sh.hyper.channel.0",
				Socket: xl.SocketInfo{Path: ctx.HyperSockName},
			},
			{
				DevId:  1,
				Name:   "sh.hyper.channel.1",
				Socket: xl.SocketInfo{Path: ctx.TtySockName},
			},
		},
		P9: []xl.P9Info{
			{
				ShareTag:      hypervisor.ShareDirTag,
				ShareDir:      ctx.ShareDir,
				SecurityModel: "none",
			},
		},
	}

	b, err := json.Marshal(config)
	if err != nil {
		glog.Errorf("fail to marshal xen domain config: %v, %v", b, err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}

	xctx := xc.driver.ctx
	domid, err := xctx.CreateNewDomainFromJson(string(b))
	if err != nil {
		glog.Errorf("fail to create xen pv domain: %v", err)
		xctx.DestroyDomainByName(ctx.Id)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}

	glog.Infof("create success, domid: %v\n", domid)

	xc.domId = xl.Domid(domid)

	go func() {
		//unpause dom until runv connected to the socket, otherwise may loss the ready message
		ctx.WaitSockConnected()
		xctx.DomainUnpause(xc.domId)
	}()
}

func (xc *XenPvContext) Associate(ctx *hypervisor.VmContext) {

}

func (xc *XenPvContext) Dump() (map[string]interface{}, error) {
	return nil, nil
}

func (xc *XenPvContext) Shutdown(ctx *hypervisor.VmContext) {
	go xc.driver.ctx.DestroyDomain(xc.domId)
}

func (xc *XenPvContext) Kill(ctx *hypervisor.VmContext) {
	go func() {
		xc.Shutdown(ctx)
		ctx.Hub <- &hypervisor.VmKilledEvent{Success: true}
	}()
}

func (xc *XenPvContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (xc *XenPvContext) Close() {}

func (xc *XenPvContext) Pause(ctx *hypervisor.VmContext, pause bool) error {
	if pause {
		return xc.driver.ctx.DomainPause(xc.domId)
	}
	return xc.driver.ctx.DomainUnpause(xc.domId)
}

func xvdId2Name(id int) string {
	return "xvd" + utils.DiskId2Name(id)
}

func (xc *XenPvContext) AddDisk(ctx *hypervisor.VmContext, sourceType string, blockInfo *hypervisor.DiskDescriptor, result chan<- hypervisor.VmEvent) {
	if blockInfo.Format == "rbd" {
		result <- &hypervisor.DeviceFailed{
			Session: nil,
		}
		glog.Infof("xenpv driver doesn't support rbd device")
		return
	}
	devName := xvdId2Name(blockInfo.ScsiId)
	if err := xc.driver.ctx.DomainAddDisk(xc.domId, blockInfo.Filename, devName, !blockInfo.ReadOnly); err != nil {
		result <- &hypervisor.DeviceFailed{
			Session: nil,
		}
		return
	}
	result <- &hypervisor.BlockdevInsertedEvent{
		DeviceName: devName,
	}
}

func (xc *XenPvContext) RemoveDisk(ctx *hypervisor.VmContext, blockInfo *hypervisor.DiskDescriptor, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	if err := xc.driver.ctx.DomainRemoveDisk(xc.domId, xvdId2Name(blockInfo.ScsiId)); err != nil {
		result <- &hypervisor.DeviceFailed{
			Session: callback,
		}
		return
	}

	result <- callback
}

func (xc *XenPvContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	callback := &hypervisor.NetDevInsertedEvent{
		Id:         host.Id,
		Index:      guest.Index,
		DeviceName: guest.Device,
		Address:    guest.Busaddr,
	}

	if err := xc.driver.ctx.DomainAddNic(xc.domId, guest.Index, host.Bridge, host.Device, host.Mac); err != nil {
		result <- &hypervisor.DeviceFailed{
			Session: callback,
		}
		return
	}
	result <- callback
}

func (xc *XenPvContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	if err := xc.driver.ctx.DomainRemoveNic(xc.domId, n.Index); err != nil {
		result <- &hypervisor.DeviceFailed{
			Session: callback,
		}
		return
	}

	result <- callback
}

func (xc *XenPvContext) SetCpus(ctx *hypervisor.VmContext, cpus int) error {
	return fmt.Errorf("SetCpus is unsupported on xenpv driver")
}

func (xc *XenPvContext) AddMem(ctx *hypervisor.VmContext, slot, size int) error {
	return fmt.Errorf("AddMem is unsupported on xenpv driver")
}

func (xc *XenPvContext) Save(ctx *hypervisor.VmContext, path string) error {
	return fmt.Errorf("Save is unsupported on xenpv driver")
}

func (xc *XenPvContext) ConnectConsole(console chan<- string) error {
	reader, writer := io.Pipe()
	args := []string{"console", "-t", "pv", fmt.Sprintf("%d", xc.domId)}
	cmd := exec.Command(xc.driver.executable, args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("fail to connect to console of dom %d: %v\n", xc.domId, err)
	}

	go func() {
		data := make([]byte, 128)
		for {
			nr, err := reader.Read(data)
			if err != nil {
				glog.Errorf("fail to read console: %v", err)
				break
			}
			console <- string(data[:nr])
		}
		reader.Close()
		writer.Close()
		cmd.Wait()
	}()

	return nil
}
