package hypervisor

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

type VmHwStatus struct {
	PciAddr  int    //next available pci addr for pci hotplug
	ScsiId   int    //next available scsi id for scsi hotplug
	AttachId uint64 //next available attachId for attached tty
}

const (
	PauseStateUnpaused = iota
	PauseStateBusy
	PauseStatePaused
)

type VmContext struct {
	Id string

	PauseState int
	Boot       *BootConfig

	vmHyperstartAPIVersion uint32

	// Communication Context
	Hub    chan VmEvent
	client chan *types.VmResponse
	vm     chan *hyperstartCmd

	DCtx DriverContext

	HomeDir         string
	HyperSockName   string
	TtySockName     string
	ConsoleSockName string
	ShareDir        string

	pciAddr int //next available pci addr for pci hotplug
	scsiId  int //next available scsi id for scsi hotplug

	InterfaceCount int

	ptys *pseudoTtys

	// Specification
	userSpec *pod.UserPod
	vmSpec   *hyperstartapi.Pod
	vmExec   map[string]*hyperstartapi.ExecCommand
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

func InitContext(id string, hub chan VmEvent, client chan *types.VmResponse, dc DriverContext, boot *BootConfig) (*VmContext, error) {
	var err error = nil

	vmChannel := make(chan *hyperstartCmd, 128)

	//dir and sockets:
	homeDir := BaseDir + "/" + id + "/"
	hyperSockName := homeDir + HyperSockName
	ttySockName := homeDir + TtySockName
	consoleSockName := homeDir + ConsoleSockName
	shareDir := homeDir + ShareDirTag

	if dc == nil {
		dc = HDriver.InitContext(homeDir)
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
		PauseState:      PauseStateUnpaused,
		pciAddr:         PciAddrFrom,
		scsiId:          0,
		Hub:             hub,
		client:          client,
		DCtx:            dc,
		vm:              vmChannel,
		ptys:            newPts(),
		HomeDir:         homeDir,
		HyperSockName:   hyperSockName,
		TtySockName:     ttySockName,
		ConsoleSockName: consoleSockName,
		ShareDir:        shareDir,
		InterfaceCount:  InterfaceCount,
		timer:           nil,
		handler:         stateInit,
		current:         StateInit,
		userSpec:        nil,
		vmSpec:          nil,
		vmExec:          make(map[string]*hyperstartapi.ExecCommand),
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

	ctx.ptys.closePendingTtys()

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

func (ctx *VmContext) LookupExecBySession(session uint64) string {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	for id, exec := range ctx.vmExec {
		if exec.Process.Stdio == session {
			glog.V(1).Infof("found exec %s whose session is %v", id, session)
			return id
		}
	}

	return ""
}

func (ctx *VmContext) DeleteExec(id string) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	delete(ctx.vmExec, id)
}

func (ctx *VmContext) LookupBySession(session uint64) string {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.vmSpec == nil {
		return ""
	}
	for idx, c := range ctx.vmSpec.Containers {
		if c.Process.Stdio == session {
			glog.V(1).Infof("found container %s whose session is %v at %d", c.Id, session, idx)
			return c.Id
		}
	}
	glog.V(1).Infof("can not found container whose session is %s", session)
	return ""
}

func (ctx *VmContext) Lookup(container string) int {
	if container == "" || ctx.vmSpec == nil {
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
	ctx.ptys.closePendingTtys()
	ctx.unsetTimeout()
	ctx.DCtx.Close()
	close(ctx.vm)
	close(ctx.client)
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
	cInfo []*ContainerInfo, vInfo map[string]*VolumeInfo) {

	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	/* Update interface count accourding to user pod */
	ret := len(spec.Interfaces)
	if ret != 0 {
		ctx.InterfaceCount = ret
	}

	for i := 0; i < ctx.InterfaceCount; i++ {
		ctx.progress.adding.networks[i] = true
	}

	if cInfo == nil {
		cInfo = []*ContainerInfo{}
	}

	if vInfo == nil {
		vInfo = make(map[string]*VolumeInfo)
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

	containers := make([]hyperstartapi.Container, len(spec.Containers))

	for i, container := range spec.Containers {
		ctx.initContainerInfo(i, &containers[i], &container)
		ctx.setContainerInfo(i, &containers[i], cInfo[i])

		containers[i].Process.Stdio = ctx.ptys.attachId
		ctx.ptys.attachId++
		if !container.Tty {
			containers[i].Process.Stderr = ctx.ptys.attachId
			ctx.ptys.attachId++
		}
	}

	hostname := spec.Hostname
	if len(hostname) == 0 {
		hostname = spec.Name
	}
	if len(hostname) > 64 {
		hostname = spec.Name[:64]
	}

	vmspec := &hyperstartapi.Pod{
		Hostname:   hostname,
		Containers: containers,
		Dns:        spec.Dns,
		Interfaces: nil,
		Routes:     nil,
		ShareDir:   ShareDirTag,
	}
	if spec.PortmappingWhiteLists != nil {
		vmspec.PortmappingWhiteLists = &hyperstartapi.PortmappingWhiteList{
			InternalNetworks: spec.PortmappingWhiteLists.InternalNetworks,
			ExternalNetworks: spec.PortmappingWhiteLists.ExternalNetworks,
		}
	}
	ctx.vmSpec = vmspec

	for _, vol := range vInfo {
		ctx.setVolumeInfo(vol)
	}

	ctx.userSpec = spec
	ctx.wg = wg
}
