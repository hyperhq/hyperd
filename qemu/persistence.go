package qemu

import (
    "encoding/json"
    "hyper/pod"
    "hyper/types"
    "errors"
    "os"
)

type PersistVolumeInfo struct {
    Name        string
    Filename    string
    Format      string
    Fstype      string
    DeviceName  string
    ScsiId      int
    Containers  []int
    MontPoints  []string
}

type PersistNetworkInfo struct {
    Index       int
    PciAddr     int
    DeviceName  string
    IpAddr      string
}

type PersistInfo struct {
    Id          string
    Pid         int
    UserSpec    *pod.UserPod
    VmSpec      *VmPod
    HwStat      *VmHwStatus
    VolumeList  []*PersistVolumeInfo
    NetworkList []*PersistNetworkInfo
}

func (ctx *VmContext) dump() (*PersistInfo, error) {
    info := &PersistInfo{
        Id: ctx.Id,
        UserSpec: ctx.userSpec,
        VmSpec:   ctx.vmSpec,
        HwStat:   ctx.dumpHwInfo(),
        VolumeList: make([]*PersistVolumeInfo, len(ctx.devices.imageMap) + len(ctx.devices.volumeMap)),
        NetworkList: make([]*PersistNetworkInfo, len(ctx.devices.networkMap)),
    }

    if ctx.process == nil {
        return nil,errors.New("No process id available")
    }
    info.Pid = ctx.process.Pid

    vid := 0
    for _,image := range ctx.devices.imageMap {
        info.VolumeList[vid] = image.info.dump()
        info.VolumeList[vid].Containers = []int{image.pos}
        info.VolumeList[vid].MontPoints = []string{"/"}
        vid++
    }

    for _,vol := range ctx.devices.volumeMap {
        info.VolumeList[vid] = vol.info.dump()
        mps := len(vol.pos)
        info.VolumeList[vid].Containers = make([]int, mps)
        info.VolumeList[vid].MontPoints = make([]string, mps)
        i := 0
        for idx,mp := range vol.pos {
            info.VolumeList[vid].Containers[i] = idx
            info.VolumeList[vid].MontPoints[i] = mp
            i++
        }
        vid++
    }

    nid := 0
    for _,nic := range ctx.devices.networkMap {
        info.NetworkList[nid] = &PersistNetworkInfo{
            Index: nic.Index,
            PciAddr: nic.PCIAddr,
            DeviceName: nic.DeviceName,
            IpAddr: nic.IpAddr,
        }
        nid++
    }

    return info,nil
}

func (ctx *VmContext) dumpHwInfo() *VmHwStatus {
    return &VmHwStatus{
        PciAddr:    ctx.pciAddr,
        ScsiId:     ctx.scsiId,
        AttachId:   ctx.attachId,
    }
}

func (ctx *VmContext) loadHwStatus(pinfo *PersistInfo) {
    ctx.pciAddr  = pinfo.HwStat.PciAddr
    ctx.scsiId   = pinfo.HwStat.ScsiId
    ctx.attachId = pinfo.HwStat.AttachId
}

func (blk *blockDescriptor) dump() *PersistVolumeInfo {
    return &PersistVolumeInfo{
        Name: blk.name,
        Filename: blk.filename,
        Format: blk.format,
        Fstype: blk.fstype,
        DeviceName: blk.deviceName,
        ScsiId: blk.scsiId,
    }
}

func (vol *PersistVolumeInfo) blockInfo() *blockDescriptor{
    return &blockDescriptor{
        name:       vol.Name,
        filename:   vol.Filename,
        format:     vol.Format,
        fstype:     vol.Fstype,
        deviceName: vol.DeviceName,
        scsiId:     vol.ScsiId,
    }
}

func (cr *VmContainer) roLookup(mpoint string) bool {
    if v := cr.volLookup(mpoint); v != nil {
        return v.ReadOnly
    } else if m:= cr.mapLookup(mpoint); m != nil {
        return m.ReadOnly
    }

    return false
}

func (cr *VmContainer) mapLookup(mpoint string) *VmFsmapDescriptor {
    for _,fs := range cr.Fsmap {
        if fs.Path == mpoint {
            return &fs
        }
    }
    return nil
}

func (cr *VmContainer) volLookup(mpoint string) *VmVolumeDescriptor {
   for _,vol := range cr.Volumes {
        if vol.Mount == mpoint {
            return &vol
        }
    }
    return nil
}

func vmDeserialize(s []byte) (*PersistInfo, error) {
    info := &PersistInfo{}
    err := json.Unmarshal(s, info)
    return info,err
}

func (pinfo *PersistInfo) serialize() ([]byte, error) {
    return json.Marshal(pinfo)
}


func (pinfo *PersistInfo) vmContext(hub chan QemuEvent, client chan *types.QemuResponse) (*VmContext, error) {

    proc,err := os.FindProcess(pinfo.Pid)
    if err != nil {
        return nil, err
    }

    ctx,err := initContext(pinfo.Id, hub, client, &BootConfig{})
    if err != nil {
        return nil, err
    }

    ctx.process     = proc
    ctx.vmSpec      = pinfo.VmSpec
    ctx.userSpec    = pinfo.UserSpec

    ctx.loadHwStatus(pinfo)

    for idx,container := range ctx.vmSpec.Containers {
        ctx.ptys.ttys[container.Tty] = newAttachments(idx, true)
    }

    for _,vol := range pinfo.VolumeList {
        binfo := vol.blockInfo()
        if len(vol.Containers) != len(vol.MontPoints) {
            return nil, errors.New("persistent data corrupt, volume info mismatch")
        }
        if len(vol.MontPoints) == 1 && vol.MontPoints[0] == "/" {
            img := &imageInfo{
                info: binfo,
                pos:  vol.Containers[0],
            }
            ctx.devices.imageMap[vol.Name] = img
        } else {
            v := &volumeInfo{
                info: binfo,
                pos:  make(map[int]string),
                readOnly: make(map[int]bool),
            }
            for i := 0; i < len(vol.Containers); i++ {
                idx := vol.Containers[i]
                v.pos[idx] = vol.MontPoints[i]
                v.readOnly[idx] = ctx.vmSpec.Containers[idx].roLookup(vol.MontPoints[i])
            }
        }
    }

    for _,nic := range pinfo.NetworkList {
        ctx.devices.networkMap[nic.Index] = &InterfaceCreated{
            Index:      nic.Index,
            PCIAddr:    nic.PciAddr,
            DeviceName: nic.DeviceName,
            IpAddr:     nic.IpAddr,
        }
    }

    return ctx,nil
}
