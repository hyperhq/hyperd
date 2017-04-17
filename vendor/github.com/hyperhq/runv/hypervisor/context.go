package hypervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
)

type VmHwStatus struct {
	PciAddr  int    //next available pci addr for pci hotplug
	ScsiId   int    //next available scsi id for scsi hotplug
	AttachId uint64 //next available attachId for attached tty
	GuestCid uint32 //vsock guest cid
}

const (
	PauseStateUnpaused = iota
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

	DCtx DriverContext

	HomeDir         string
	HyperSockName   string
	TtySockName     string
	ConsoleSockName string
	ShareDir        string
	GuestCid        uint32

	pciAddr int //next available pci addr for pci hotplug
	scsiId  int //next available scsi id for scsi hotplug

	//	InterfaceCount int

	hyperstart libhyperstart.Hyperstart

	// Specification
	volumes    map[string]*DiskContext
	containers map[string]*ContainerContext
	networks   *NetworkContext

	// internal states
	vmExec map[string]*hyperstartapi.ExecCommand

	// Internal Helper
	handler stateHandler
	current string
	timer   *time.Timer

	logPrefix string

	lock      sync.RWMutex //protect update of context
	idLock    sync.Mutex
	pauseLock sync.Mutex
	closeOnce sync.Once
}

type stateHandler func(ctx *VmContext, event VmEvent)

func NewVmSpec() *hyperstartapi.Pod {
	return &hyperstartapi.Pod{
		ShareDir: ShareDirTag,
	}
}

func InitContext(id string, hub chan VmEvent, client chan *types.VmResponse, dc DriverContext, boot *BootConfig) (*VmContext, error) {
	var (
		//dir and sockets:
		homeDir         = filepath.Join(BaseDir, id)
		hyperSockName   = filepath.Join(homeDir, HyperSockName)
		ttySockName     = filepath.Join(homeDir, TtySockName)
		consoleSockName = filepath.Join(homeDir, ConsoleSockName)
		shareDir        = filepath.Join(homeDir, ShareDirTag)
		ctx             *VmContext
		cid             uint32
	)

	err := os.MkdirAll(shareDir, 0755)
	if err != nil {
		ctx.Log(ERROR, "cannot make dir %s: %v", shareDir, err)
		return nil, err
	}

	if dc == nil {
		dc = HDriver.InitContext(homeDir)
		if dc == nil {
			err := fmt.Errorf("cannot create driver context of %s", homeDir)
			ctx.Log(ERROR, "init failed: %v", err)
			return nil, err
		}
	}

	if boot.EnableVsock {
		if !HDriver.SupportVmSocket() {
			err := fmt.Errorf("vsock feature requested but not supported")
			ctx.Log(ERROR, "%v", err)
			return nil, err
		}
		cid, err = VsockCidManager.GetCid()
		if err != nil {
			ctx.Log(ERROR, "failed to get vsock guest cid: %v", err)
			return nil, err
		}
	}

	ctx = &VmContext{
		Id:              id,
		Boot:            boot,
		PauseState:      PauseStateUnpaused,
		pciAddr:         PciAddrFrom,
		scsiId:          0,
		GuestCid:        cid,
		Hub:             hub,
		client:          client,
		DCtx:            dc,
		HomeDir:         homeDir,
		HyperSockName:   hyperSockName,
		TtySockName:     ttySockName,
		ConsoleSockName: consoleSockName,
		ShareDir:        shareDir,
		timer:           nil,
		handler:         stateRunning,
		current:         StateRunning,
		volumes:         make(map[string]*DiskContext),
		containers:      make(map[string]*ContainerContext),
		networks:        NewNetworkContext(),
		vmExec:          make(map[string]*hyperstartapi.ExecCommand),
		logPrefix:       fmt.Sprintf("SB[%s] ", id),
	}
	ctx.networks.sandbox = ctx

	return ctx, nil
}

// SendVmEvent enqueues a VmEvent onto the context. Returns an error if there is
// no handler associated with the context. VmEvent handling happens in a
// separate goroutine, so this is thread-safe and asynchronous.
func (ctx *VmContext) SendVmEvent(ev VmEvent) error {
	ctx.lock.RLock()
	defer ctx.lock.RUnlock()

	if ctx.handler == nil {
		return fmt.Errorf("VmContext(%s): event handler already shutdown.", ctx.Id)
	}

	ctx.Hub <- ev

	return nil
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

func (ctx *VmContext) nextScsiId() int {
	ctx.idLock.Lock()
	id := ctx.scsiId
	ctx.scsiId++
	ctx.idLock.Unlock()
	return id
}

func (ctx *VmContext) NextPciAddr() int {
	ctx.idLock.Lock()
	addr := ctx.pciAddr
	ctx.pciAddr++
	ctx.idLock.Unlock()
	return addr
}

func (ctx *VmContext) LookupExecBySession(session uint64) string {
	ctx.lock.RLock()
	defer ctx.lock.RUnlock()

	for id, exec := range ctx.vmExec {
		if exec.Process.Stdio == session {
			ctx.Log(DEBUG, "found exec %s whose session is %v", id, session)
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
	ctx.lock.RLock()
	defer ctx.lock.RUnlock()

	for id, c := range ctx.containers {
		if c.process.Stdio == session {
			ctx.Log(DEBUG, "found container %s whose session is %v", c.Id, session)
			return id
		}
	}
	ctx.Log(DEBUG, "can not found container whose session is %s", session)
	return ""
}

func (ctx *VmContext) Close() {
	ctx.closeOnce.Do(func() {
		ctx.Log(INFO, "VmContext Close()")
		ctx.lock.Lock()
		defer ctx.lock.Unlock()
		ctx.unsetTimeout()
		ctx.networks.close()
		ctx.DCtx.Close()
		ctx.hyperstart.Close()
		close(ctx.client)
		os.Remove(ctx.ShareDir)
		ctx.handler = nil
		ctx.current = "None"
		if ctx.Boot.EnableVsock && ctx.GuestCid > 0 {
			VsockCidManager.ReleaseCid(ctx.GuestCid)
			ctx.GuestCid = 0
		}
	})
}

func (ctx *VmContext) Become(handler stateHandler, desc string) {
	orig := ctx.current
	ctx.lock.Lock()
	ctx.handler = handler
	ctx.current = desc
	ctx.lock.Unlock()
	ctx.Log(DEBUG, "state change from %s to '%s'", orig, desc)
}

func (ctx *VmContext) IsRunning() bool {
	var running bool
	ctx.lock.RLock()
	running = ctx.current == StateRunning
	ctx.lock.RUnlock()
	return running
}

// User API
func (ctx *VmContext) SetNetworkEnvironment(net *api.SandboxConfig) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	ctx.networks.SandboxConfig = net
}

func (ctx *VmContext) AddPortmapping(ports []*api.PortDescription) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
}

func (ctx *VmContext) AddInterface(inf *api.InterfaceDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "add interface %s during %v", inf.Id, ctx.current)
		result <- NewNotReadyError(ctx.Id)
	}

	ctx.networks.addInterface(inf, result)
}

func (ctx *VmContext) RemoveInterface(id string, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "remove interface %s during %v", id, ctx.current)
		result <- api.NewResultBase(id, true, "pod not running")
	}

	ctx.networks.removeInterface(id, result)
}

func (ctx *VmContext) validateContainer(c *api.ContainerDescription) error {
	for vn, vr := range c.Volumes {
		if _, ok := ctx.volumes[vn]; !ok {
			return fmt.Errorf("volume %s does not exist in volume map", vn)
		}
		for _, mp := range vr.MountPoints {
			path := filepath.Clean(mp.Path)
			if path == "/" {
				return fmt.Errorf("mounting volume %s to rootfs is forbidden", vn)
			}
		}
	}

	return nil
}

func (ctx *VmContext) AddContainer(c *api.ContainerDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "add container %s during %v", c.Id, ctx.current)
		result <- NewNotReadyError(ctx.Id)
	}

	if ctx.LogLevel(TRACE) {
		ctx.Log(TRACE, "add container %#v", c)
	}

	if _, ok := ctx.containers[c.Id]; ok {
		estr := fmt.Sprintf("duplicate container %s", c.Name)
		ctx.Log(ERROR, estr)
		result <- NewSpecError(c.Id, estr)
		return
	}
	cc := &ContainerContext{
		ContainerDescription: c,
		fsmap:                []*hyperstartapi.FsmapDescriptor{},
		vmVolumes:            []*hyperstartapi.VolumeDescriptor{},
		sandbox:              ctx,
		logPrefix:            fmt.Sprintf("SB[%s] Con[%s] ", ctx.Id, c.Id),
	}

	wgDisk := &sync.WaitGroup{}
	added := []string{}
	rollback := func() {
		for _, d := range added {
			ctx.volumes[d].unwait(c.Id)
		}
	}

	if err := ctx.validateContainer(c); err != nil {
		cc.Log(ERROR, err.Error())
		result <- NewSpecError(c.Id, err.Error())
		return
	}

	for vn := range c.Volumes {
		entry, ok := ctx.volumes[vn]
		if !ok {
			estr := fmt.Sprintf("volume %s does not exist in volume map", vn)
			cc.Log(ERROR, estr)
			rollback()
			result <- NewSpecError(c.Id, estr)
			return
		}

		entry.wait(c.Id, wgDisk)
		added = append(added, vn)
	}

	//prepare runtime environment
	cc.configProcess()

	cc.root = NewDiskContext(ctx, c.RootVolume)
	cc.root.isRootVol = true
	cc.root.insert(nil)
	cc.root.wait(c.Id, wgDisk)

	ctx.containers[c.Id] = cc

	go cc.add(wgDisk, result)

	return
}

func (ctx *VmContext) RemoveContainer(id string, result chan<- api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "remove container %s during %v", id, ctx.current)
		result <- api.NewResultBase(id, true, "pod not running")
	}

	cc, ok := ctx.containers[id]
	if !ok {
		ctx.Log(WARNING, "container %s not exist", id)
		result <- api.NewResultBase(id, true, "not exist")
		return
	}

	for v := range cc.Volumes {
		if vol, ok := ctx.volumes[v]; ok {
			vol.unwait(id)
		}
	}

	cc.root.unwait(id)

	ctx.Log(INFO, "remove container %s", id)
	delete(ctx.containers, id)
	cc.root.remove(result)
}

func (ctx *VmContext) AddVolume(vol *api.VolumeDescription, result chan api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "add volume %s during %v", vol.Name, ctx.current)
		result <- NewNotReadyError(ctx.Id)
	}

	if _, ok := ctx.volumes[vol.Name]; ok {
		estr := fmt.Sprintf("duplicate volume %s", vol.Name)
		ctx.Log(WARNING, estr)
		result <- api.NewResultBase(vol.Name, true, estr)
		return
	}

	dc := NewDiskContext(ctx, vol)

	if vol.IsDir() || vol.IsNas() {
		ctx.Log(INFO, "return volume add success for dir/nas %s", vol.Name)
		result <- api.NewResultBase(vol.Name, true, "")
	} else {
		ctx.Log(DEBUG, "insert disk for volume %s", vol.Name)
		dc.insert(result)
	}

	ctx.volumes[vol.Name] = dc
}

func (ctx *VmContext) RemoveVolume(name string, result chan<- api.Result) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if ctx.current != StateRunning {
		ctx.Log(DEBUG, "remove container %s during %v", name, ctx.current)
		result <- api.NewResultBase(name, true, "pod not running")
	}

	disk, ok := ctx.volumes[name]
	if !ok {
		ctx.Log(WARNING, "volume %s not exist", name)
		result <- api.NewResultBase(name, true, "not exist")
		return
	}

	if disk.containers() > 0 {
		ctx.Log(ERROR, "cannot remove a in use volume %s", name)
		result <- api.NewResultBase(name, false, "in use")
		return
	}

	ctx.Log(INFO, "remove disk %s", name)
	delete(ctx.volumes, name)
	disk.remove(result)
}

func (ctx *VmContext) ctlSockAddr() string {
	if ctx.Boot.EnableVsock {
		return utils.VSOCK_SOCKET_PREFIX + strconv.FormatUint(uint64(ctx.GuestCid), 10) + ":" + strconv.FormatInt(hyperstartapi.HYPER_VSOCK_CTL_PORT, 10)
	} else {
		return utils.UNIX_SOCKET_PREFIX + ctx.HyperSockName
	}
}

func (ctx *VmContext) ttySockAddr() string {
	if ctx.Boot.EnableVsock {
		return utils.VSOCK_SOCKET_PREFIX + strconv.FormatUint(uint64(ctx.GuestCid), 10) + ":" + strconv.FormatInt(hyperstartapi.HYPER_VSOCK_MSG_PORT, 10)
	} else {
		return utils.UNIX_SOCKET_PREFIX + ctx.TtySockName
	}
}
