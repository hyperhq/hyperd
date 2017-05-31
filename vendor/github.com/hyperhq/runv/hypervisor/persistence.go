package hypervisor

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
)

const CURRENT_PERSIST_VERSION = 20170224

type PersistVolumeInfo struct {
	Name         string
	Filename     string
	Format       string
	Fstype       string
	DeviceName   string
	ScsiId       int
	ContainerIds []string
	IsRootVol    bool
	Containers   []int // deprecated
	MontPoints   []string
}

type PersistNetworkInfo struct {
	Id         string
	Index      int
	PciAddr    int
	DeviceName string
	IpAddr     string
}

type PersistInfo struct {
	PersistVersion int
	Id             string
	DriverInfo     map[string]interface{}
	VmSpec         *hyperstartapi.Pod
	HwStat         *VmHwStatus
	VolumeList     []*PersistVolumeInfo
	NetworkList    []*PersistNetworkInfo
	PortList       []*api.PortDescription
}

func (p *PersistInfo) LogLevel(level hlog.LogLevel) bool {
	return hlog.IsLogLevel(level)
}

func (p *PersistInfo) LogPrefix() string {
	return fmt.Sprintf("PSB[%s] ", p.Id)
}

func (p *PersistInfo) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, p, 1, args...)
}

func (ctx *VmContext) dump() (*PersistInfo, error) {
	dr, err := ctx.DCtx.Dump()
	if err != nil {
		return nil, err
	}

	nc := ctx.networks
	info := &PersistInfo{
		PersistVersion: CURRENT_PERSIST_VERSION,
		Id:             ctx.Id,
		DriverInfo:     dr,
		VmSpec:         ctx.networks.sandboxInfo(),
		HwStat:         ctx.dumpHwInfo(),
		VolumeList:     make([]*PersistVolumeInfo, len(ctx.volumes)+len(ctx.containers)),
		NetworkList:    make([]*PersistNetworkInfo, len(nc.eth)+len(nc.lo)),
		PortList:       make([]*api.PortDescription, len(nc.ports)),
	}

	vid := 0
	for _, vol := range ctx.volumes {
		v := vol.dump()
		for id := range vol.observers {
			v.ContainerIds = append(v.ContainerIds, id)
		}
		info.VolumeList[vid] = v
		vid++
	}

	for i, p := range nc.ports {
		info.PortList[i] = &api.PortDescription{
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      p.Protocol,
		}
	}
	nid := 0
	for _, nic := range nc.lo {
		info.NetworkList[nid] = &PersistNetworkInfo{
			Id:         nic.Id,
			Index:      nic.Index,
			PciAddr:    nic.PCIAddr,
			DeviceName: nic.DeviceName,
			IpAddr:     nic.IpAddr,
		}
		nid++
	}
	nc.slotLock.RLock()
	for _, nic := range nc.eth {
		info.NetworkList[nid] = &PersistNetworkInfo{
			Id:         nic.Id,
			Index:      nic.Index,
			PciAddr:    nic.PCIAddr,
			DeviceName: nic.DeviceName,
			IpAddr:     nic.IpAddr,
		}
		nid++
	}
	defer nc.slotLock.RUnlock()

	cid := 0
	info.VmSpec.DeprecatedContainers = make([]hyperstartapi.Container, len(ctx.containers))
	for _, c := range ctx.containers {
		info.VmSpec.DeprecatedContainers[cid] = *c.VmSpec()
		rootVolume := c.root.dump()
		rootVolume.ContainerIds = []string{c.Id}
		rootVolume.IsRootVol = true
		info.VolumeList[vid] = rootVolume
		vid++
		cid++
	}

	return info, nil
}

func (ctx *VmContext) dumpHwInfo() *VmHwStatus {
	return &VmHwStatus{
		PciAddr:  ctx.pciAddr,
		ScsiId:   ctx.scsiId,
		AttachId: ctx.hyperstart.LastStreamSeq(),
		GuestCid: ctx.GuestCid,
	}
}

func (ctx *VmContext) loadHwStatus(pinfo *PersistInfo) error {
	ctx.pciAddr = pinfo.HwStat.PciAddr
	ctx.scsiId = pinfo.HwStat.ScsiId
	ctx.GuestCid = pinfo.HwStat.GuestCid
	if ctx.GuestCid != 0 {
		if !VsockCidManager.MarkCidInuse(ctx.GuestCid) {
			return fmt.Errorf("conflicting vsock guest cid %d: already in use", ctx.GuestCid)
		}
		ctx.Boot.EnableVsock = true
	}
	return nil
}

func (blk *DiskDescriptor) dump() *PersistVolumeInfo {
	return &PersistVolumeInfo{
		Name:       blk.Name,
		Filename:   blk.Filename,
		Format:     blk.Format,
		Fstype:     blk.Fstype,
		DeviceName: blk.DeviceName,
		ScsiId:     blk.ScsiId,
	}
}

func (vol *PersistVolumeInfo) blockInfo() *DiskDescriptor {
	return &DiskDescriptor{
		Name:       vol.Name,
		Filename:   vol.Filename,
		Format:     vol.Format,
		Fstype:     vol.Fstype,
		DeviceName: vol.DeviceName,
		ScsiId:     vol.ScsiId,
	}
}

func (nc *NetworkContext) load(pinfo *PersistInfo) {
	nc.SandboxConfig = &api.SandboxConfig{
		Hostname: pinfo.VmSpec.Hostname,
		Dns:      pinfo.VmSpec.Dns,
	}
	portWhilteList := pinfo.VmSpec.PortmappingWhiteLists
	if portWhilteList != nil {
		nc.Neighbors = &api.NeighborNetworks{
			InternalNetworks: portWhilteList.InternalNetworks,
			ExternalNetworks: portWhilteList.ExternalNetworks,
		}
	}

	for i, p := range pinfo.PortList {
		nc.ports[i] = p
	}
	for _, pi := range pinfo.NetworkList {
		ifc := &InterfaceCreated{
			Id:         pi.Id,
			Index:      pi.Index,
			PCIAddr:    pi.PciAddr,
			DeviceName: pi.DeviceName,
			IpAddr:     pi.IpAddr,
		}
		// if empty, may be old data, generate one for compatibility.
		if ifc.Id == "" {
			ifc.Id = utils.RandStr(8, "alpha")
		}
		// use device name distinguish from lo and eth
		if ifc.DeviceName == DEFAULT_LO_DEVICE_NAME {
			nc.lo[pi.IpAddr] = ifc
		} else {
			nc.eth[pi.Index] = ifc
		}
		nc.idMap[pi.Id] = ifc
	}
}

func vmDeserialize(s []byte) (*PersistInfo, error) {
	info := &PersistInfo{}
	err := json.Unmarshal(s, info)
	return info, err
}

func (pinfo *PersistInfo) serialize() ([]byte, error) {
	return json.Marshal(pinfo)
}

func (pinfo *PersistInfo) vmContext(hub chan VmEvent, client chan *types.VmResponse) (*VmContext, error) {
	oldVersion := pinfo.PersistVersion < CURRENT_PERSIST_VERSION

	dc, err := HDriver.LoadContext(pinfo.DriverInfo)
	if err != nil {
		pinfo.Log(ERROR, "cannot load driver context: %v", err)
		return nil, err
	}

	ctx, err := InitContext(pinfo.Id, hub, client, dc, &BootConfig{})
	if err != nil {
		return nil, err
	}

	err = ctx.loadHwStatus(pinfo)
	if err != nil {
		return nil, err
	}

	ctx.networks.load(pinfo)

	// map container id to image DiskDescriptor
	imageMap := make(map[string]*DiskDescriptor)
	// map container id to volume DiskContext list
	volumeMap := make(map[string][]*DiskContext)
	pcList := pinfo.VmSpec.DeprecatedContainers
	for _, vol := range pinfo.VolumeList {
		binfo := vol.blockInfo()
		if oldVersion {
			if len(vol.Containers) != len(vol.MontPoints) {
				return nil, fmt.Errorf("persistent data corrupt, volume info mismatch")
			}
			if len(vol.MontPoints) == 1 && vol.MontPoints[0] == "/" {
				imageMap[pcList[vol.Containers[0]].Id] = binfo
				continue
			}
		} else {
			if vol.IsRootVol {
				if len(vol.ContainerIds) != 1 {
					return nil, fmt.Errorf("persistent data corrupt, root volume mismatch")
				}
				imageMap[vol.ContainerIds[0]] = binfo
				continue
			}
		}
		ctx.volumes[binfo.Name] = &DiskContext{
			DiskDescriptor: binfo,
			sandbox:        ctx,
			observers:      make(map[string]*sync.WaitGroup),
			lock:           &sync.RWMutex{},
			// FIXME: is there any trouble if we set it as ready when restoring from persistence
			ready: true,
		}
		if oldVersion {
			for _, idx := range vol.Containers {
				volumeMap[pcList[idx].Id] = append(volumeMap[pcList[idx].Id], ctx.volumes[binfo.Name])
			}
		} else {
			for _, id := range vol.ContainerIds {
				volumeMap[id] = append(volumeMap[id], ctx.volumes[binfo.Name])
			}
		}
	}

	for _, pc := range pcList {
		bInfo, ok := imageMap[pc.Id]
		if !ok {
			return nil, fmt.Errorf("persistent data corrupt, lack of container root volume")
		}
		cc := &ContainerContext{
			ContainerDescription: &api.ContainerDescription{
				Id:         pc.Id,
				RootPath:   pc.Rootfs,
				Initialize: pc.Initialize,
				Sysctl:     pc.Sysctl,
				RootVolume: &api.VolumeDescription{
					Name:         bInfo.Name,
					Source:       bInfo.Filename,
					Format:       bInfo.Format,
					Fstype:       bInfo.Fstype,
					DockerVolume: bInfo.DockerVolume,
					ReadOnly:     bInfo.ReadOnly,
				},
			},
			fsmap:     pc.Fsmap,
			process:   pc.Process,
			vmVolumes: pc.Volumes,
			sandbox:   ctx,
			logPrefix: fmt.Sprintf("SB[%s] Con[%s] ", ctx.Id, pc.Id),
			root: &DiskContext{
				DiskDescriptor: bInfo,
				sandbox:        ctx,
				isRootVol:      true,
				ready:          true,
				observers:      make(map[string]*sync.WaitGroup),
				lock:           &sync.RWMutex{},
			},
		}
		if cc.process.Id == "" {
			cc.process.Id = "init"
		}
		// restore wg for volumes attached to container
		wgDisk := &sync.WaitGroup{}
		volList, ok := volumeMap[pc.Id]
		if ok {
			cc.Volumes = make(map[string]*api.VolumeReference, len(volList))
			for _, vol := range volList {
				// for unwait attached volumes when removing container
				cc.Volumes[vol.Name] = &api.VolumeReference{}
				vol.wait(cc.Id, wgDisk)
			}
		}
		cc.root.wait(cc.Id, wgDisk)

		ctx.containers[cc.Id] = cc
	}

	return ctx, nil
}
