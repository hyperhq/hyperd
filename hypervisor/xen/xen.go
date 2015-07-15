package xen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/hyperhq/hyper/hypervisor"
	"github.com/hyperhq/hyper/lib/glog"
	"github.com/hyperhq/hyper/network"
	"net"
	"strings"
	"unsafe"
)

type XenDriver struct {
	Ctx          LibxlCtxPtr
	Version      uint32
	Capabilities string
	Logger       *XentoollogLogger
	domains      map[uint32]*hypervisor.VmContext
}

type XenContext struct {
	driver *XenDriver
	domId  int
	ev     unsafe.Pointer
}

type DomainConfig struct {
	Hvm         bool
	Name        string
	Kernel      string
	Initrd      string
	Cmdline     string
	MaxVcpus    int
	MaxMemory   int
	ConsoleSock string
	Extra       []string
}

var globalDriver *XenDriver = nil

func InitDriver() *XenDriver {
	xd := &XenDriver{}
	if err := xd.Initialize(); err == nil {
		glog.Info("Xen Driver Loaded.")
		globalDriver = xd
		return globalDriver
	} else {
		glog.Info("Xen Driver Load failed: ", err.Error())
		return nil
	}
}

//judge if the xl is available and if the version and cap is acceptable

func (xd *XenDriver) Initialize() error {

	if probeXend() {
		return errors.New("xend is running, can not start with xl.")
	}

	ctx, res := HyperxlInitializeDriver()
	if res != 0 {
		return errors.New("failed to initialize xen context")
	} else if ctx.Version < REQUIRED_VERSION {
		return fmt.Errorf("Xen version is not new enough (%d), need 4.5 or higher", ctx.Version)
	} else {
		glog.V(1).Info("Xen capabilities: ", ctx.Capabilities)
		hvm := false
		caps := strings.Split(ctx.Capabilities, " ")
		for _, cap := range caps {
			if strings.HasPrefix(cap, "hvm-") {
				hvm = true
				break
			}
		}
		if !hvm {
			return fmt.Errorf("Xen installation does not support HVM, current capabilities: %s", ctx.Capabilities)
		}
	}

	sigchan := make(chan os.Signal, 1)
	go func() {
		for {
			_, ok := <-sigchan
			if !ok {
				break
			}
			glog.V(1).Info("got SIGCHLD, send msg to libxl")
			HyperxlSigchldHandler(ctx.Ctx)
		}
	}()
	signal.Notify(sigchan, syscall.SIGCHLD)

	xd.Ctx = ctx.Ctx
	xd.Logger = ctx.Logger
	xd.Version = ctx.Version
	xd.Capabilities = ctx.Capabilities
	xd.domains = make(map[uint32]*hypervisor.VmContext)

	return nil
}

func (xd *XenDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &XenContext{driver: xd, domId: -1}
}

func (xd *XenDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "xen" {
		return nil, errors.New("wrong driver type in persist info")
	}

	var domid int

	d, ok := persisted["domid"]
	if !ok {
		return nil, errors.New("cannot read the dom id info from persist info")
	} else {
		switch d.(type) {
		case float64:
			domid = (int)(d.(float64))
			if domid <= 0 {
				return nil, fmt.Errorf("loaded wrong domid %d", domid)
			}
			if HyperxlDomainCheck(xd.Ctx, (uint32)(domid)) != 0 {
				return nil, fmt.Errorf("cannot load domain %d, not exist", domid)
			}
		default:
			return nil, errors.New("wrong domid type in persist info")
		}
	}

	return &XenContext{driver: xd, domId: domid}, nil
}

func (xc *XenContext) Launch(ctx *hypervisor.VmContext) {
	//    go func(){
	extra := []string{
		"-device", fmt.Sprintf("virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=%d", PCI_AVAILABLE_ADDRESS),
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	}
	domid, ev, err := XlStartDomain(xc.driver.Ctx, ctx.Id, ctx.Boot, ctx.HyperSockName+".test", ctx.TtySockName+".test", ctx.ConsoleSockName, extra)
	if err != nil {
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: err.Error()}
		return
	}
	xc.domId = domid
	xc.ev = ev
	glog.Infof("Start VM as domain %d", domid)
	xc.driver.domains[(uint32)(domid)] = ctx
	//    }()
}

func (xc *XenContext) Associate(ctx *hypervisor.VmContext) {
	xc.driver.domains[(uint32)(xc.domId)] = ctx
}

func (xc *XenContext) Dump() (map[string]interface{}, error) {
	if xc.domId <= 0 {
		return nil, fmt.Errorf("Dom id is invalid: %d", xc.domId)
	}

	return map[string]interface{}{
		"hypervisor": "xen",
		"domid":      xc.domId,
	}, nil
}

func (xc *XenContext) Shutdown(ctx *hypervisor.VmContext) {
	go func() {
		res := HyperxlDomainDestroy(xc.driver.Ctx, (uint32)(xc.domId))
		if res == 0 {
			ctx.Hub <- &hypervisor.VmExit{}
		}
		if xc.ev != unsafe.Pointer(nil) {
			HyperDomainCleanup(xc.driver.Ctx, xc.ev)
		}
	}()
}

func (xc *XenContext) Kill(ctx *hypervisor.VmContext) {
	go func() {
		res := HyperxlDomainDestroy(xc.driver.Ctx, (uint32)(xc.domId))
		ctx.Hub <- &hypervisor.VmKilledEvent{Success: res == 0}
	}()
}

func (xc *XenContext) BuildinNetwork() bool { return false }

func (xc *XenContext) Close() {}

func (xc *XenContext) AddDisk(ctx *hypervisor.VmContext, name, sourceType, filename, format string, id int) {
	go diskRoutine(true, xc, ctx, name, sourceType, filename, format, id, nil)
}

func (xc *XenContext) RemoveDisk(ctx *hypervisor.VmContext, filename, format string, id int, callback hypervisor.VmEvent) {
	go diskRoutine(false, xc, ctx, "", "", filename, format, id, callback)
}

func (xc *XenContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
	go func() {
		callback := &hypervisor.NetDevInsertedEvent{
			Index:      guest.Index,
			DeviceName: guest.Device,
			Address:    guest.Busaddr,
		}

		glog.V(1).Infof("allocate nic %s for dom %d", host.Mac, xc.domId)
		hw, err := net.ParseMAC(host.Mac)
		if err == nil {
			//dev := fmt.Sprintf("vif%d.%d", xc.domId, guest.Index)
			dev := host.Device
			glog.V(1).Infof("add network for %d - ip: %s, br: %s, gw: %s, dev: %s, hw: %s", xc.domId, guest.Ipaddr,
				host.Bridge, host.Bridge, dev, hw.String())

			res := HyperxlNicAdd(xc.driver.Ctx, (uint32)(xc.domId), guest.Ipaddr, host.Bridge, host.Bridge, dev, []byte(hw))
			if res == 0 {

				glog.V(1).Infof("nic %s insert succeeded", guest.Device)

				err = network.UpAndAddToBridge(fmt.Sprintf("vif%d.%d", xc.domId, guest.Index))
				if err != nil {
					glog.Error("fail to add vif to bridge: ", err.Error())
					ctx.Hub <- &hypervisor.DeviceFailed{
						Session: callback,
					}
					HyperxlNicRemove(xc.driver.Ctx, (uint32)(xc.domId), host.Mac)
					return
				}

				ctx.Hub <- callback
				return
			}
			glog.V(1).Infof("nic %s insert succeeded [faked] ", guest.Device)
			ctx.Hub <- callback
			return
		}

		glog.Errorf("nic %s insert failed", guest.Device)
		ctx.Hub <- &hypervisor.DeviceFailed{
			Session: callback,
		}
	}()
}

func (xc *XenContext) RemoveNic(ctx *hypervisor.VmContext, device, mac string, callback hypervisor.VmEvent) {
	go func() {
		res := HyperxlNicRemove(xc.driver.Ctx, (uint32)(xc.domId), mac)
		if res == 0 {
			glog.V(1).Infof("nic %s remove succeeded", device)
			ctx.Hub <- callback
			return
		}
		glog.Errorf("nic %s remove failed", device)
		ctx.Hub <- &hypervisor.DeviceFailed{
			Session: callback,
		}
	}()
}

func diskRoutine(add bool, xc *XenContext, ctx *hypervisor.VmContext,
	name, sourceType, filename, format string, id int, callback hypervisor.VmEvent) {
	backend := LIBXL_DISK_BACKEND_TAP
	if strings.HasPrefix(filename, "/dev/") {
		backend = LIBXL_DISK_BACKEND_PHY
	}
	dfmt := LIBXL_DISK_FORMAT_RAW
	if format == "qcow" || format == "qcow2" {
		dfmt = LIBXL_DISK_FORMAT_QCOW2
	}

	devName := xvdId2Name(id)
	var res int
	var op string = "insert"
	if add {
		res = HyperxlDiskAdd(xc.driver.Ctx, uint32(xc.domId), filename, devName, LibxlDiskBackend(backend), LibxlDiskFormat(dfmt))
		callback = &hypervisor.BlockdevInsertedEvent{
			Name:       name,
			SourceType: sourceType,
			DeviceName: devName,
			ScsiId:     id,
		}
	} else {
		op = "remove"
		res = HyperxlDiskRemove(xc.driver.Ctx, uint32(xc.domId), filename, devName, LibxlDiskBackend(backend), LibxlDiskFormat(dfmt))
	}
	if res == 0 {
		glog.V(1).Infof("Disk %s (%s) %s succeeded", devName, filename, op)
		ctx.Hub <- callback
		return
	}

	glog.Errorf("Disk %s (%s) insert %s failed", devName, filename, op)
	ctx.Hub <- &hypervisor.DeviceFailed{
		Session: callback,
	}
}

func XlStartDomain(ctx LibxlCtxPtr, id string, boot *hypervisor.BootConfig, hyperSock, ttySock, consoleSock string, extra []string) (int, unsafe.Pointer, error) {

	config := &DomainConfig{
		Hvm:         true,
		Name:        id,
		Kernel:      boot.Kernel,
		Initrd:      boot.Initrd,
		Cmdline:     "console=ttyS0 pci=nomsi",
		MaxVcpus:    boot.CPU,
		MaxMemory:   boot.Memory << 10,
		ConsoleSock: fmt.Sprintf("unix:%s,server,nowait", consoleSock),
		Extra:       extra,
	}

	domid, ev, err := HyperxlDomainStart(ctx, config)
	if err != 0 {
		return -1, nil, errors.New("failed to start a xen domain.")
	}

	return domid, ev, nil
}

func probeXend() bool {
	xend, err := exec.LookPath("xend")
	if err != nil {
		return false
	}

	cmd := exec.Command(xend, "status")
	err = cmd.Run()
	return err == nil
}

func xvdId2Name(id int) string {
	return "xvd" + hypervisor.DiskId2Name(id)
}
