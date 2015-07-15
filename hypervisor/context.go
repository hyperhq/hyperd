package hypervisor

import (
	"encoding/json"
	"github.com/hyperhq/hyper/lib/glog"
	"github.com/hyperhq/hyper/pod"
	"github.com/hyperhq/hyper/types"
	"os"
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
	Hub    chan VmEvent
	client chan *types.QemuResponse
	vm     chan *DecodedMessage

	DCtx DriverContext

	HomeDir         string
	HyperSockName   string
	TtySockName     string
	ConsoleSockName string
	ShareDir        string

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

	lock *sync.Mutex //protect update of context
	wg   *sync.WaitGroup
	wait bool
}

type stateHandler func(ctx *VmContext, event VmEvent)

func InitContext(dr HypervisorDriver, id string, hub chan VmEvent, client chan *types.QemuResponse, dc DriverContext, boot *BootConfig) (*VmContext, error) {

	var err error = nil

	vmChannel := make(chan *DecodedMessage, 128)

	//dir and sockets:
	homeDir := BaseDir + "/" + id + "/"
	hyperSockName := homeDir + HyperSockName
	ttySockName := homeDir + TtySockName
	consoleSockName := homeDir + ConsoleSockName
	shareDir := homeDir + ShareDirTag

	if dc == nil {
		dc = dr.InitContext(homeDir)
	}

	err = os.MkdirAll(shareDir, 0755)
	if err != nil {
		glog.Error("cannot make dir", shareDir, err.Error())
		return nil, err
	}
	defer func() {
		if err != nil {
			os.Remove(homeDir)
		}
	}()

	return &VmContext{
		Id:              id,
		Boot:            boot,
		pciAddr:         PciAddrFrom,
		scsiId:          0,
		attachId:        1,
		Hub:             hub,
		client:          client,
		DCtx:            dc,
		vm:              vmChannel,
		ptys:            newPts(),
		ttySessions:     make(map[string]uint64),
		HomeDir:         homeDir,
		HyperSockName:   hyperSockName,
		TtySockName:     ttySockName,
		ConsoleSockName: consoleSockName,
		ShareDir:        shareDir,
		timer:           nil,
		handler:         stateInit,
		userSpec:        nil,
		vmSpec:          nil,
		devices:         newDeviceMap(),
		progress:        newProcessingList(),
		lock:            &sync.Mutex{},
		wait:            false,
	}, nil
}

func (ctx *VmContext) setTimeout(seconds int) {
	if ctx.timer != nil {
		ctx.unsetTimeout()
	}
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		ctx.Hub <- &VmTimeout{}
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
	ctx.DCtx.Close()
	close(ctx.vm)
	os.Remove(ctx.ShareDir)
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
