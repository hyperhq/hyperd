package hypervisor

import (
	"fmt"
	"hyper/lib/glog"
	"hyper/pod"
)

type deviceMap struct {
	imageMap   map[string]*imageInfo
	volumeMap  map[string]*volumeInfo
	networkMap map[int]*InterfaceCreated
}

type blockDescriptor struct {
	name       string
	filename   string
	format     string
	fstype     string
	deviceName string
	scsiId     int
}

type imageInfo struct {
	info *blockDescriptor
	pos  int
}

type volumeInfo struct {
	info     *blockDescriptor
	pos      volumePosition
	readOnly map[int]bool
}

type volumePosition map[int]string //containerIdx -> mpoint

type processingList struct {
	adding   *processingMap
	deleting *processingMap
	finished *processingMap
}

type processingMap struct {
	containers  map[int]bool
	volumes     map[string]bool
	blockdevs   map[string]bool
	networks    map[int]bool
	ttys        map[int]bool
	serialPorts map[int]bool
}

func newProcessingMap() *processingMap {
	return &processingMap{
		containers: make(map[int]bool),    //to be create, and get images,
		volumes:    make(map[string]bool), //to be create, and get volume
		blockdevs:  make(map[string]bool), //to be insert to qemu, both volume and images
		networks:   make(map[int]bool),
	}
}

func newProcessingList() *processingList {
	return &processingList{
		adding:   newProcessingMap(),
		deleting: newProcessingMap(),
		finished: newProcessingMap(),
	}
}

func newDeviceMap() *deviceMap {
	return &deviceMap{
		imageMap:   make(map[string]*imageInfo),
		volumeMap:  make(map[string]*volumeInfo),
		networkMap: make(map[int]*InterfaceCreated),
	}
}

func (pm *processingMap) isEmpty() bool {
	return len(pm.containers) == 0 && len(pm.volumes) == 0 && len(pm.blockdevs) == 0 &&
		len(pm.networks) == 0
}

func (ctx *VmContext) deviceReady() bool {
	return ctx.progress.adding.isEmpty() && ctx.progress.deleting.isEmpty()
}

func (ctx *VmContext) initContainerInfo(index int, target *VmContainer, spec *pod.UserContainer) {
	vols := []VmVolumeDescriptor{}
	fsmap := []VmFsmapDescriptor{}
	for _, v := range spec.Volumes {
		ctx.devices.volumeMap[v.Volume].pos[index] = v.Path
		ctx.devices.volumeMap[v.Volume].readOnly[index] = v.ReadOnly
	}

	envs := make([]VmEnvironmentVar, len(spec.Envs))
	for j, e := range spec.Envs {
		envs[j] = VmEnvironmentVar{Env: e.Env, Value: e.Value}
	}

	restart := "never"
	if len(spec.RestartPolicy) > 0 {
		restart = spec.RestartPolicy
	}

	*target = VmContainer{
		Id: "", Rootfs: "rootfs", Fstype: "", Image: "",
		Volumes: vols, Fsmap: fsmap, Tty: 0,
		Workdir: spec.Workdir, Entrypoint: spec.Entrypoint, Cmd: spec.Command, Envs: envs,
		RestartPolicy: restart,
	}
}

func (ctx *VmContext) setContainerInfo(index int, container *VmContainer, info *ContainerInfo) {

	container.Id = info.Id
	container.Rootfs = info.Rootfs

	cmd := container.Entrypoint
	if len(container.Entrypoint) == 0 && len(info.Entrypoint) > 0 {
		cmd = info.Entrypoint
	}
	if len(container.Cmd) > 0 {
		cmd = append(cmd, container.Cmd...)
	} else if len(info.Cmd) > 0 {
		cmd = append(cmd, info.Cmd...)
	}
	container.Cmd = cmd
	container.Entrypoint = []string{}

	if container.Workdir == "" {
		if info.Workdir != "" {
			container.Workdir = info.Workdir
		} else {
			container.Workdir = "/"
		}
	}

	for _, e := range container.Envs {
		if _, ok := info.Envs[e.Env]; ok {
			delete(info.Envs, e.Env)
		}
	}
	for e, v := range info.Envs {
		container.Envs = append(container.Envs, VmEnvironmentVar{Env: e, Value: v})
	}

	if info.Fstype == "dir" {
		container.Image = info.Image
		container.Fstype = ""
	} else {
		container.Fstype = info.Fstype
		ctx.devices.imageMap[info.Image] = &imageInfo{
			info: &blockDescriptor{
				name: info.Image, filename: info.Image, format: "raw", fstype: info.Fstype, deviceName: ""},
			pos: index,
		}
		ctx.progress.adding.blockdevs[info.Image] = true
	}
}

func (ctx *VmContext) initVolumeMap(spec *pod.UserPod) {
	//classify volumes, and generate device info and progress info
	for _, vol := range spec.Volumes {
		if vol.Source == "" || vol.Driver == "" {
			ctx.devices.volumeMap[vol.Name] = &volumeInfo{
				info:     &blockDescriptor{name: vol.Name, filename: "", format: "", fstype: "", deviceName: ""},
				pos:      make(map[int]string),
				readOnly: make(map[int]bool),
			}
		} else if vol.Driver == "raw" || vol.Driver == "qcow2" {
			ctx.devices.volumeMap[vol.Name] = &volumeInfo{
				info: &blockDescriptor{
					name: vol.Name, filename: vol.Source, format: vol.Driver, fstype: "ext4", deviceName: ""},
				pos:      make(map[int]string),
				readOnly: make(map[int]bool),
			}
			ctx.progress.adding.blockdevs[vol.Name] = true
		} else if vol.Driver == "vfs" {
			ctx.devices.volumeMap[vol.Name] = &volumeInfo{
				info: &blockDescriptor{
					name: vol.Name, filename: vol.Source, format: vol.Driver, fstype: "dir", deviceName: ""},
				pos:      make(map[int]string),
				readOnly: make(map[int]bool),
			}
		}
	}
}

func (ctx *VmContext) setVolumeInfo(info *VolumeInfo) {

	vol, ok := ctx.devices.volumeMap[info.Name]
	if !ok {
		return
	}

	vol.info.filename = info.Filepath
	vol.info.format = info.Format

	if info.Fstype != "dir" {
		vol.info.fstype = info.Fstype
		ctx.progress.adding.blockdevs[info.Name] = true
	} else {
		vol.info.fstype = ""
		for i, mount := range vol.pos {
			glog.V(1).Infof("insert volume %s to %s on %d", info.Name, mount, i)
			ctx.vmSpec.Containers[i].Fsmap = append(ctx.vmSpec.Containers[i].Fsmap, VmFsmapDescriptor{
				Source:   info.Filepath,
				Path:     mount,
				ReadOnly: vol.readOnly[i],
			})
		}
	}
}

func (ctx *VmContext) allocateNetworks() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for i, _ := range ctx.progress.adding.networks {
		name := fmt.Sprintf("eth%d", i)
		addr := ctx.nextPciAddr()
		go CreateInterface(i, addr, name, i == 0, maps, ctx.hub)
	}
}

func (ctx *VmContext) addBlockDevices() {
	for blk, _ := range ctx.progress.adding.blockdevs {
		if info, ok := ctx.devices.volumeMap[blk]; ok {
			sid := ctx.nextScsiId()
			newDiskAddSession(ctx, info.info.name, "volume", info.info.filename, info.info.format, sid)
		} else if info, ok := ctx.devices.imageMap[blk]; ok {
			sid := ctx.nextScsiId()
			newDiskAddSession(ctx, info.info.name, "image", info.info.filename, info.info.format, sid)
		} else {
			continue
		}
	}
}

func (ctx *VmContext) blockdevInserted(info *BlockdevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if info.SourceType == "image" {
		image := ctx.devices.imageMap[info.Name]
		image.info.deviceName = info.DeviceName
		image.info.scsiId = info.ScsiId
		ctx.vmSpec.Containers[image.pos].Image = info.DeviceName
	} else if info.SourceType == "volume" {
		volume := ctx.devices.volumeMap[info.Name]
		volume.info.deviceName = info.DeviceName
		volume.info.scsiId = info.ScsiId
		for c, vol := range volume.pos {
			ctx.vmSpec.Containers[c].Volumes = append(ctx.vmSpec.Containers[c].Volumes,
				VmVolumeDescriptor{
					Device:   info.DeviceName,
					Mount:    vol,
					Fstype:   volume.info.fstype,
					ReadOnly: volume.readOnly[c],
				})
		}
	}

	ctx.progress.finished.blockdevs[info.Name] = true
	if _, ok := ctx.progress.adding.blockdevs[info.Name]; ok {
		delete(ctx.progress.adding.blockdevs, info.Name)
	}
}

func (ctx *VmContext) interfaceCreated(info *InterfaceCreated) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.devices.networkMap[info.Index] = info
}

func (ctx *VmContext) netdevInserted(info *NetDevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.progress.finished.networks[info.Index] = true
	if _, ok := ctx.progress.adding.networks[info.Index]; ok {
		delete(ctx.progress.adding.networks, info.Index)
	}
	if len(ctx.progress.adding.networks) == 0 {
		count := len(ctx.devices.networkMap)
		infs := make([]VmNetworkInf, count)
		routes := []VmRoute{}
		for i := 0; i < count; i++ {
			infs[i].Device = ctx.devices.networkMap[i].DeviceName
			infs[i].IpAddress = ctx.devices.networkMap[i].IpAddr
			infs[i].NetMask = ctx.devices.networkMap[i].NetMask

			for _, rl := range ctx.devices.networkMap[i].RouteTable {
				dev := ""
				if rl.ViaThis {
					dev = infs[i].Device
				}
				routes = append(routes, VmRoute{
					Dest:    rl.Destination,
					Gateway: rl.Gateway,
					Device:  dev,
				})
			}
		}
		ctx.vmSpec.Interfaces = infs
		ctx.vmSpec.Routes = routes
	}
}

func (ctx *VmContext) onContainerRemoved(c *ContainerUnmounted) bool {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if _, ok := ctx.progress.deleting.containers[c.Index]; ok {
		glog.V(1).Infof("container %d umounted", c.Index)
		delete(ctx.progress.deleting.containers, c.Index)
	}
	if ctx.vmSpec.Containers[c.Index].Fstype != "" {
		for name, image := range ctx.devices.imageMap {
			if image.pos == c.Index {
				glog.V(1).Info("need remove image dm file", image.info.filename)
				ctx.progress.deleting.blockdevs[name] = true
				go UmountDMDevice(image.info.filename, name, ctx.hub)
			}
		}
	}

	return c.Success
}

func (ctx *VmContext) onInterfaceRemoved(nic *InterfaceReleased) bool {
	if _, ok := ctx.progress.deleting.networks[nic.Index]; ok {
		glog.V(1).Infof("interface %d released", nic.Index)
		delete(ctx.progress.deleting.networks, nic.Index)
	}

	return nic.Success
}

func (ctx *VmContext) onVolumeRemoved(v *VolumeUnmounted) bool {
	if _, ok := ctx.progress.deleting.volumes[v.Name]; ok {
		glog.V(1).Infof("volume %s umounted", v.Name)
		delete(ctx.progress.deleting.volumes, v.Name)
	}
	vol := ctx.devices.volumeMap[v.Name]
	if vol.info.fstype != "" {
		glog.V(1).Info("need remove dm file ", vol.info.filename)
		ctx.progress.deleting.blockdevs[vol.info.name] = true
		go UmountDMDevice(vol.info.filename, vol.info.name, ctx.hub)
	}
	return v.Success
}

func (ctx *VmContext) onBlockReleased(v *BlockdevRemovedEvent) bool {
	if _, ok := ctx.progress.deleting.blockdevs[v.Name]; ok {
		glog.V(1).Infof("blockdev %s deleted", v.Name)
		delete(ctx.progress.deleting.blockdevs, v.Name)
	}
	return v.Success
}

func (ctx *VmContext) releaseVolumeDir() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.fstype == "" {
			glog.V(1).Info("need umount dir ", vol.info.filename)
			ctx.progress.deleting.volumes[name] = true
			go UmountVolume(ctx.shareDir, vol.info.filename, name, ctx.hub)
		}
	}
}

func (ctx *VmContext) removeDMDevice() {
	for name, container := range ctx.devices.imageMap {
		if container.info.fstype != "dir" {
			glog.V(1).Info("need remove dm file", container.info.filename)
			ctx.progress.deleting.blockdevs[name] = true
			go UmountDMDevice(container.info.filename, name, ctx.hub)
		}
	}
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.fstype != "" {
			glog.V(1).Info("need remove dm file ", vol.info.filename)
			ctx.progress.deleting.blockdevs[name] = true
			go UmountDMDevice(vol.info.filename, name, ctx.hub)
		}
	}
}

func (ctx *VmContext) releaseOverlayDir() {
	for idx, container := range ctx.vmSpec.Containers {
		if container.Fstype == "" {
			glog.V(1).Info("need unmount overlay dir ", container.Image)
			ctx.progress.deleting.containers[idx] = true
			go UmountOverlayContainer(ctx.shareDir, container.Image, idx, ctx.hub)
		}
	}
}

func (ctx *VmContext) releaseAufsDir() {
	for idx, container := range ctx.vmSpec.Containers {
		if container.Fstype == "" {
			glog.V(1).Info("need unmount aufs ", container.Image)
			ctx.progress.deleting.containers[idx] = true
			go UmountAufsContainer(ctx.shareDir, container.Image, idx, ctx.hub)
		}
	}
}

func (ctx *VmContext) removeVolumeDrive() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.format == "raw" || vol.info.format == "qcow2" {
			glog.V(1).Infof("need detach volume %s (%s) ", name, vol.info.deviceName)
			newDiskDelSession(ctx, vol.info.scsiId, &VolumeUnmounted{Name: name, Success: true})
			ctx.progress.deleting.volumes[name] = true
		}
	}
}

func (ctx *VmContext) removeImageDrive() {
	for _, image := range ctx.devices.imageMap {
		if image.info.fstype != "dir" {
			glog.V(1).Infof("need eject no.%d image block device: %s", image.pos, image.info.deviceName)
			ctx.progress.deleting.containers[image.pos] = true
			newDiskDelSession(ctx, image.info.scsiId, &ContainerUnmounted{Index: image.pos, Success: true})
		}
	}
}

func (ctx *VmContext) releaseNetwork() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for idx, nic := range ctx.devices.networkMap {
		glog.V(1).Infof("remove network card %d: %s", idx, nic.IpAddr)
		ctx.progress.deleting.networks[idx] = true
		ReleaseInterface(idx, nic.IpAddr, nic.Fd, maps, ctx.hub)
		maps = nil
	}
}

func (ctx *VmContext) removeInterface() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for idx, nic := range ctx.devices.networkMap {
		glog.V(1).Infof("remove network card %d: %s", idx, nic.IpAddr)
		ctx.progress.deleting.networks[idx] = true
		ReleaseInterface(idx, nic.IpAddr, nic.Fd, maps, ctx.hub)
		newNetworkDelSession(ctx, nic.DeviceName, &NetDevRemovedEvent{Index: idx})
		maps = nil
	}
}
