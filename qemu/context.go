package qemu

import (
	"encoding/json"
	"fmt"
	"hyper/lib/glog"
	"hyper/pod"
	"hyper/types"
	"os"
	"strconv"
	"sync"
	"time"
)

type VmOnDiskInfo struct {
	QmpSockName     string
	HyperSockName   string
	TtySockName     string
	ConsoleSockName string
	ShareDir        string
}

type VmHwStatus struct {
	PciAddr  int    //next available pci addr for pci hotplug
	ScsiId   int    //next available scsi id for scsi hotplug
	AttachId uint64 //next available attachId for attached tty
}

type VmContext struct {
	Id string

	Boot *BootConfig

	// Communication Context
	hub    chan QemuEvent
	client chan *types.QemuResponse
	vm     chan *DecodedMessage

	qmp chan QmpInteraction
	wdt chan string

	qmpSockName     string
	hyperSockName   string
	ttySockName     string
	consoleSockName string
	shareDir        string

	pciAddr  int    //next available pci addr for pci hotplug
	scsiId   int    //next available scsi id for scsi hotplug
	attachId uint64 //next available attachId for attached tty

	ptys        *pseudoTtys
	ttySessions map[string]uint64

	// Specification
	userSpec *pod.UserPod
	vmSpec   *VmPod
	devices  *deviceMap

	progress *processingList

	// Internal Helper
	handler stateHandler
	current string
	timer   *time.Timer
	process *os.Process
	lock    *sync.Mutex //protect update of context
	wg	*sync.WaitGroup
	wait	bool
}

type stateHandler func(ctx *VmContext, event QemuEvent)

func initContext(id string, hub chan QemuEvent, client chan *types.QemuResponse, boot *BootConfig) (*VmContext, error) {

	var err error = nil

	qmpChannel := make(chan QmpInteraction, 128)
	vmChannel := make(chan *DecodedMessage, 128)
	defer func() {
		if err != nil {
			close(qmpChannel)
			close(vmChannel)
		}
	}()

	//dir and sockets:
	homeDir := BaseDir + "/" + id + "/"
	qmpSockName := homeDir + QmpSockName
	hyperSockName := homeDir + HyperSockName
	ttySockName := homeDir + TtySockName
	consoleSockName := homeDir + ConsoleSockName
	shareDir := homeDir + ShareDirTag

	err = os.MkdirAll(shareDir, 0755)
	if err != nil {
		glog.Error("cannot make dir", shareDir, err.Error())
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(homeDir)
		}
	}()

	return &VmContext{
		Id:              id,
		Boot:            boot,
		pciAddr:         PciAddrFrom,
		scsiId:          0,
		attachId:        1,
		hub:             hub,
		client:          client,
		qmp:             qmpChannel,
		vm:              vmChannel,
		wdt:             make(chan string, 16),
		ptys:            newPts(),
		ttySessions:     make(map[string]uint64),
		qmpSockName:     qmpSockName,
		hyperSockName:   hyperSockName,
		ttySockName:     ttySockName,
		consoleSockName: consoleSockName,
		shareDir:        shareDir,
		timer:           nil,
		process:         nil,
		handler:         stateInit,
		userSpec:        nil,
		vmSpec:          nil,
		devices:         newDeviceMap(),
		progress:        newProcessingList(),
		lock:            &sync.Mutex{},
		wait:		 false,
	}, nil
}

func (ctx *VmContext) setTimeout(seconds int) {
	if ctx.timer != nil {
		ctx.unsetTimeout()
	}
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		ctx.hub <- &QemuTimeout{}
	})
}

func (ctx *VmContext) unsetTimeout() {
	if ctx.timer != nil {
		ctx.timer.Stop()
		ctx.timer = nil
	}
}

func (ctx *VmContext) reset() {
	ctx.lock.Lock()

	ctx.pciAddr = PciAddrFrom
	ctx.scsiId = 0
	//do not reset attach id here, let it increase

	ctx.userSpec = nil
	ctx.vmSpec = nil
	ctx.devices = newDeviceMap()
	ctx.progress = newProcessingList()

	ctx.lock.Unlock()
}

func (ctx *VmContext) nextScsiId() int {
	ctx.lock.Lock()
	id := ctx.scsiId
	ctx.scsiId++
	ctx.lock.Unlock()
	return id
}

func (ctx *VmContext) nextPciAddr() int {
	ctx.lock.Lock()
	addr := ctx.pciAddr
	ctx.pciAddr++
	ctx.lock.Unlock()
	return addr
}

func (ctx *VmContext) nextAttachId() uint64 {
	ctx.lock.Lock()
	id := ctx.attachId
	ctx.attachId++
	ctx.lock.Unlock()
	return id
}

func (ctx *VmContext) clientReg(tag string, session uint64) {
	ctx.lock.Lock()
	ctx.ttySessions[tag] = session
	ctx.lock.Unlock()
}

func (ctx *VmContext) clientDereg(tag string) {
	if tag == "" {
		return
	}
	ctx.lock.Lock()
	if _, ok := ctx.ttySessions[tag]; ok {
		delete(ctx.ttySessions, tag)
	}
	ctx.lock.Unlock()
}

func (ctx *VmContext) Lookup(container string) int {
	if container == "" {
		return -1
	}
	for idx, c := range ctx.vmSpec.Containers {
		if c.Id == container {
			glog.V(1).Infof("found container %s at %d", container, idx)
			return idx
		}
	}
	glog.V(1).Infof("can not found container %s", container)
	return -1
}

func (ctx *VmContext) Close() {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.unsetTimeout()
	close(ctx.qmp)
	close(ctx.vm)
	close(ctx.wdt)
	os.RemoveAll(ctx.shareDir)
	ctx.handler = nil
	ctx.current = "None"
}

func (ctx *VmContext) tryClose() bool {
	if ctx.deviceReady() {
		glog.V(1).Info("no more device to release/remove/umount, quit")
		ctx.Close()
		return true
	}
	return false
}

func (ctx *VmContext) Become(handler stateHandler, desc string) {
	orig := ctx.current
	ctx.lock.Lock()
	ctx.handler = handler
	ctx.current = desc
	ctx.lock.Unlock()
	glog.V(1).Infof("VM %s: state change from %s to '%s'", ctx.Id, orig, desc)
}

func (ctx *VmContext) QemuArguments() []string {
	if ctx.Boot == nil {
		ctx.Boot = &BootConfig{
			CPU:    1,
			Memory: 128,
			Kernel: DefaultKernel,
			Initrd: DefaultInitrd,
		}
	}
	boot := ctx.Boot

	params := []string{
		"-machine", "pc-i440fx-2.0,accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host"}
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		glog.V(1).Info("kvm not exist change to no kvm mode")
		params = []string{"-machine", "pc-i440fx-2.0,usb=off", "-cpu", "core2duo"}
	}

	if boot.Bios != "" && boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Bios),
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else if boot.Bios != "" {
		params = append(params,
			"-bios", boot.Bios,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "\"console=ttyS0 panic=1\"")
	} else if boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else {
		params = append(params,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "\"console=ttyS0 panic=1\"")
	}

	return append(params,
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-no-hpet",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", strconv.Itoa(ctx.Boot.Memory), "-smp", strconv.Itoa(ctx.Boot.CPU),
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", ctx.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.consoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.hyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.ttySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.shareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", ShareDirTag),
	)
}

// InitDeviceContext will init device info in context
func (ctx *VmContext) InitDeviceContext(spec *pod.UserPod, wg *sync.WaitGroup,
					cInfo []*ContainerInfo, vInfo []*VolumeInfo) {

	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	for i := 0; i < InterfaceCount; i++ {
		ctx.progress.adding.networks[i] = true
	}

	if cInfo == nil {
		cInfo = []*ContainerInfo{}
	}

	if vInfo == nil {
		vInfo = []*VolumeInfo{}
	}

	ctx.initVolumeMap(spec)

	if glog.V(3) {
		for i, c := range cInfo {
			glog.Infof("#%d Container Info:", i)
			b, err := json.MarshalIndent(c, "...|", "    ")
			if err == nil {
				glog.Info("\n", string(b))
			}
		}
	}

	containers := make([]VmContainer, len(spec.Containers))

	for i, container := range spec.Containers {

		ctx.initContainerInfo(i, &containers[i], &container)
		ctx.setContainerInfo(i, &containers[i], cInfo[i])

		if spec.Tty {
			containers[i].Tty = ctx.attachId
			ctx.attachId++
			ctx.ptys.ttys[containers[i].Tty] = newAttachments(i, true)
		}
	}

	ctx.vmSpec = &VmPod{
		Hostname:   spec.Name,
		Containers: containers,
		Interfaces: nil,
		Routes:     nil,
		ShareDir:   ShareDirTag,
	}

	for _, vol := range vInfo {
		ctx.setVolumeInfo(vol)
	}

	ctx.userSpec = spec
	ctx.wg = wg
}
