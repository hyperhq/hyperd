package hypervisor

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

type deviceMap struct {
	imageMap   map[string]*imageInfo
	volumeMap  map[string]*volume
	networkMap map[int]*InterfaceCreated
}

type BlockDescriptor struct {
	Name       string
	Filename   string
	Format     string
	Fstype     string
	DeviceName string
	ScsiId     int
	Options    map[string]string
}

type imageInfo struct {
	info *BlockDescriptor
	pos  int
}

type volume struct {
	info         *BlockDescriptor
	pos          volumePosition
	readOnly     map[int]bool
	dockerVolume bool
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
		blockdevs:  make(map[string]bool), //to be insert to VM, both volume and images
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
		volumeMap:  make(map[string]*volume),
		networkMap: make(map[int]*InterfaceCreated),
	}
}

func (pm *processingMap) isEmpty() bool {
	return len(pm.containers) == 0 && len(pm.volumes) == 0 && len(pm.blockdevs) == 0 &&
		len(pm.networks) == 0
}

func (ctx *VmContext) deviceReady() bool {
	ready := ctx.progress.adding.isEmpty() && ctx.progress.deleting.isEmpty()
	if ready && ctx.wait {
		glog.V(1).Info("All resource being released, someone is waiting for us...")
		ctx.wg.Done()
		ctx.wait = false
	}

	return ready
}

func (ctx *VmContext) initContainerInfo(index int, target *hyperstartapi.Container, spec *pod.UserContainer) {
	vols := []hyperstartapi.VolumeDescriptor{}
	fsmap := []hyperstartapi.FsmapDescriptor{}
	for _, v := range spec.Volumes {
		ctx.devices.volumeMap[v.Volume].pos[index] = v.Path
		ctx.devices.volumeMap[v.Volume].readOnly[index] = v.ReadOnly
	}

	envs := make([]hyperstartapi.EnvironmentVar, len(spec.Envs))
	for j, e := range spec.Envs {
		envs[j] = hyperstartapi.EnvironmentVar{Env: e.Env, Value: e.Value}
	}

	restart := "never"
	if len(spec.RestartPolicy) > 0 {
		restart = spec.RestartPolicy
	}

	ports := make([]hyperstartapi.Port, 0, len(spec.Ports))
	for _, port := range spec.Ports {
		if port.ContainerPort == 0 {
			continue
		}
		p := hyperstartapi.Port{
			Protocol:      port.Protocol,
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
		}
		if port.Protocol == "" {
			p.Protocol = "tcp"
		}
		ports = append(ports, p)
	}

	p := hyperstartapi.Process{User: spec.User.Name, Group: spec.User.Group, AdditionalGroups: spec.User.AdditionalGroups,
		Terminal: spec.Tty, Stdio: 0, Stderr: 0, Args: spec.Command, Envs: envs, Workdir: spec.Workdir}
	*target = hyperstartapi.Container{
		Id: "", Rootfs: "rootfs", Fstype: "", Image: "",
		Volumes: vols, Fsmap: fsmap, Process: p, Ports: ports,
		Sysctl: spec.Sysctl, RestartPolicy: restart,
	}
}

func (ctx *VmContext) setContainerInfo(index int, container *hyperstartapi.Container, info *ContainerInfo) {

	container.Id = info.Id
	container.Rootfs = info.Rootfs

	container.Process.Args = info.Cmd
	container.Process.Envs = make([]hyperstartapi.EnvironmentVar, len(info.Envs))
	i := 0
	for e, v := range info.Envs {
		container.Process.Envs[i].Env = e
		container.Process.Envs[i].Value = v
		i++
	}

	if container.Process.User == "" && info.User != "" {
		container.Process.User = info.User
	}

	if container.Process.Workdir == "" {
		if info.Workdir != "" {
			container.Process.Workdir = info.Workdir
		} else {
			container.Process.Workdir = "/"
		}
	}

	container.Initialize = info.Initialize

	if info.Fstype == "dir" {
		container.Image = info.Image.Source
		container.Fstype = ""
	} else {
		container.Fstype = info.Fstype
		ctx.devices.imageMap[info.Image.Source] = &imageInfo{
			info: &BlockDescriptor{
				Name:       info.Image.Source,
				Filename:   info.Image.Source,
				Format:     "raw",
				Fstype:     info.Fstype,
				DeviceName: "",
				Options: map[string]string{
					"user":        info.Image.Option.User,
					"keyring":     info.Image.Option.Keyring,
					"monitors":    strings.Join(info.Image.Option.Monitors, ";"),
					"bytespersec": strconv.Itoa(info.Image.Option.BytesPerSec),
					"iops":        strconv.Itoa(info.Image.Option.Iops),
				}},
			pos: index,
		}
		glog.V(1).Infof("insert volume %s source %s fstype %s", info.Image.Source, info.Image.Source, info.Fstype)
		ctx.progress.adding.blockdevs[info.Image.Source] = true
	}
}

func (ctx *VmContext) initVolumeMap(spec *pod.UserPod) {
	//classify volumes, and generate device info and progress info
	for _, vol := range spec.Volumes {
		v := &volume{
			pos:      make(map[int]string),
			readOnly: make(map[int]bool),
		}

		if vol.Source == "" || vol.Driver == "" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   "",
				Format:     "",
				Fstype:     "",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
		} else if vol.Driver == "raw" || vol.Driver == "qcow2" || vol.Driver == "vdi" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "ext4",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
			ctx.progress.adding.blockdevs[vol.Name] = true
		} else if vol.Driver == "vfs" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "dir",
				DeviceName: "",
			}
			ctx.devices.volumeMap[vol.Name] = v
		} else if vol.Driver == "rbd" {
			v.info = &BlockDescriptor{
				Name:       vol.Name,
				Filename:   vol.Source,
				Format:     vol.Driver,
				Fstype:     "ext4",
				DeviceName: "",
				Options: map[string]string{
					"user":     vol.Option.User,
					"keyring":  vol.Option.Keyring,
					"monitors": strings.Join(vol.Option.Monitors, ";"),
				},
			}
			if vol.Option.BytesPerSec != 0 {
				bytes := strconv.Itoa(vol.Option.BytesPerSec)
				v.info.Options["bytespersec"] = bytes
			}
			if vol.Option.Iops != 0 {
				iops := strconv.Itoa(vol.Option.Iops)
				v.info.Options["iops"] = iops
			}
			ctx.devices.volumeMap[vol.Name] = v
			ctx.progress.adding.blockdevs[vol.Name] = true
		}
	}
}

func (ctx *VmContext) setVolumeInfo(info *VolumeInfo) {
	vol, ok := ctx.devices.volumeMap[info.Name]
	if !ok {
		return
	}

	vol.info.Filename = info.Filepath
	vol.info.Format = info.Format
	vol.dockerVolume = info.DockerVolume

	if info.Fstype != "dir" {
		vol.info.Fstype = info.Fstype
		ctx.progress.adding.blockdevs[info.Name] = true
	} else {
		vol.info.Fstype = ""
		for i, mount := range vol.pos {
			glog.V(1).Infof("insert volume %s to %s on %d", info.Name, mount, i)
			ctx.vmSpec.Containers[i].Fsmap = append(ctx.vmSpec.Containers[i].Fsmap, hyperstartapi.FsmapDescriptor{
				Source:       info.Filepath,
				Path:         mount,
				ReadOnly:     vol.readOnly[i],
				DockerVolume: info.DockerVolume,
			})
		}
	}
}

func (ctx *VmContext) allocateNetworks() {
	for i := range ctx.progress.adding.networks {
		name := fmt.Sprintf("eth%d", i)
		addr := ctx.nextPciAddr()
		if len(ctx.userSpec.Interfaces) > 0 {
			go ctx.ConfigureInterface(i, addr, name, ctx.userSpec.Interfaces[i], ctx.Hub)
		} else {
			go ctx.CreateInterface(i, addr, name)
		}
	}

	for _, srv := range ctx.userSpec.Services {
		inf := hyperstartapi.NetworkInf{
			Device:    "lo",
			IpAddress: srv.ServiceIP,
			NetMask:   "255.255.255.255",
		}

		ctx.vmSpec.Interfaces = append(ctx.vmSpec.Interfaces, inf)
	}
}

func (ctx *VmContext) addBlockDevices() {
	for blk := range ctx.progress.adding.blockdevs {
		if info, ok := ctx.devices.volumeMap[blk]; ok {
			sid := ctx.nextScsiId()
			info.info.ScsiId = sid
			ctx.DCtx.AddDisk(ctx, "volume", info.info)
		} else if info, ok := ctx.devices.imageMap[blk]; ok {
			sid := ctx.nextScsiId()
			info.info.ScsiId = sid
			ctx.DCtx.AddDisk(ctx, "image", info.info)
		} else {
			continue
		}
	}
}

func (ctx *VmContext) allocateDevices() {
	if len(ctx.progress.adding.networks) == 0 && len(ctx.progress.adding.blockdevs) == 0 {
		ctx.Hub <- &DevSkipEvent{}
		return
	}

	ctx.allocateNetworks()
	ctx.addBlockDevices()
}

func (ctx *VmContext) blockdevInserted(info *BlockdevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if info.SourceType == "image" {
		image := ctx.devices.imageMap[info.Name]
		image.info.DeviceName = info.DeviceName
		image.info.ScsiId = info.ScsiId
		ctx.vmSpec.Containers[image.pos].Image = info.DeviceName
		ctx.vmSpec.Containers[image.pos].Addr = info.ScsiAddr
	} else if info.SourceType == "volume" {
		volume := ctx.devices.volumeMap[info.Name]
		volume.info.DeviceName = info.DeviceName
		volume.info.ScsiId = info.ScsiId
		for c, vol := range volume.pos {
			ctx.vmSpec.Containers[c].Volumes = append(ctx.vmSpec.Containers[c].Volumes,
				hyperstartapi.VolumeDescriptor{
					Device:       info.DeviceName,
					Addr:         info.ScsiAddr,
					Mount:        vol,
					Fstype:       volume.info.Fstype,
					ReadOnly:     volume.readOnly[c],
					DockerVolume: volume.dockerVolume,
				})
		}
	}

	ctx.progress.finished.blockdevs[info.Name] = true
	if _, ok := ctx.progress.adding.blockdevs[info.Name]; ok {
		delete(ctx.progress.adding.blockdevs, info.Name)
	}
}

func (ctx *VmContext) interfaceCreated(info *InterfaceCreated, lazy bool, result chan<- VmEvent) {
	ctx.lock.Lock()
	ctx.devices.networkMap[info.Index] = info
	ctx.lock.Unlock()

	h := &HostNicInfo{
		Fd:      uint64(info.Fd.Fd()),
		Device:  info.HostDevice,
		Mac:     info.MacAddr,
		Bridge:  info.Bridge,
		Gateway: info.Bridge,
	}
	g := &GuestNicInfo{
		Device:  info.DeviceName,
		Ipaddr:  info.IpAddr,
		Index:   info.Index,
		Busaddr: info.PCIAddr,
	}

	if lazy {
		ctx.DCtx.(LazyDriverContext).LazyAddNic(ctx, h, g)
	} else {
		ctx.DCtx.AddNic(ctx, h, g, result)
	}
}

func (ctx *VmContext) netdevInserted(info *NetDevInsertedEvent) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	ctx.progress.finished.networks[info.Index] = true
	if _, ok := ctx.progress.adding.networks[info.Index]; ok {
		delete(ctx.progress.adding.networks, info.Index)
	}
	if len(ctx.progress.adding.networks) == 0 {
		for _, dev := range ctx.devices.networkMap {
			inf := hyperstartapi.NetworkInf{
				Device:    dev.DeviceName,
				IpAddress: dev.IpAddr,
				NetMask:   dev.NetMask,
			}
			ctx.vmSpec.Interfaces = append(ctx.vmSpec.Interfaces, inf)
			for _, rl := range dev.RouteTable {
				device := ""
				if rl.ViaThis {
					device = inf.Device
				}
				ctx.vmSpec.Routes = append(ctx.vmSpec.Routes, hyperstartapi.Route{
					Dest:    rl.Destination,
					Gateway: rl.Gateway,
					Device:  device,
				})
			}
		}
	}
}

func (ctx *VmContext) onContainerRemoved(c *ContainerUnmounted) bool {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if _, ok := ctx.progress.deleting.containers[c.Index]; ok {
		glog.V(1).Infof("container %d umounted", c.Index)
		delete(ctx.progress.deleting.containers, c.Index)
	}

	return c.Success
}

func (ctx *VmContext) onInterfaceRemoved(nic *InterfaceReleased) bool {
	if _, ok := ctx.progress.deleting.networks[nic.Index]; ok {
		glog.V(1).Infof("interface %d released", nic.Index)
		delete(ctx.progress.deleting.networks, nic.Index)
		delete(ctx.devices.networkMap, nic.Index)
	}

	return nic.Success
}

func (ctx *VmContext) onVolumeRemoved(v *VolumeUnmounted) bool {
	if _, ok := ctx.progress.deleting.volumes[v.Name]; ok {
		glog.V(1).Infof("volume %s umounted", v.Name)
		delete(ctx.progress.deleting.volumes, v.Name)
	}
	return v.Success
}

func (ctx *VmContext) removeVolumeDrive() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.Format == "raw" || vol.info.Format == "qcow2" || vol.info.Format == "vdi" || vol.info.Format == "rbd" {
			glog.V(1).Infof("need detach volume %s (%s) ", name, vol.info.DeviceName)
			ctx.DCtx.RemoveDisk(ctx, vol.info, &VolumeUnmounted{Name: name, Success: true})
			ctx.progress.deleting.volumes[name] = true
		}
	}
}

func (ctx *VmContext) removeImageDrive() {
	for _, image := range ctx.devices.imageMap {
		if image.info.Fstype != "dir" {
			glog.V(1).Infof("need eject no.%d image block device: %s", image.pos, image.info.DeviceName)
			ctx.progress.deleting.containers[image.pos] = true
			ctx.DCtx.RemoveDisk(ctx, image.info, &ContainerUnmounted{Index: image.pos, Success: true})
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
		go ctx.ReleaseInterface(idx, nic.IpAddr, nic.Fd, maps)
		maps = nil
	}
}

func (ctx *VmContext) releaseNetworkByLinkIndex(index int) {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	nic, ok := ctx.devices.networkMap[index]
	if !ok {
		glog.Error("trying to remove an un exist card:", nic)
		return
	}

	if ctx.progress.deleting.networks[index] == false {
		glog.V(1).Infof("remove network card %d: %s", index, nic.IpAddr)
		go ctx.ReleaseInterface(index, nic.IpAddr, nic.Fd, maps)
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
		ctx.DCtx.RemoveNic(ctx, nic, &NetDevRemovedEvent{Index: idx})
		maps = nil
	}
}

func (ctx *VmContext) removeInterfaceByLinkIndex(index int) {
	nic, ok := ctx.devices.networkMap[index]
	if !ok {
		glog.Error("trying to remove an un exist card:", nic)
		return
	}

	if ctx.progress.deleting.networks[index] == false {
		glog.V(1).Infof("remove network card %d: %s", index, nic.IpAddr)
		ctx.progress.deleting.networks[index] = true
		ctx.DCtx.RemoveNic(ctx, nic, &NetDevRemovedEvent{Index: index})
	}
}

func (ctx *VmContext) GetNextNicName(result chan<- string) {
	nameList := []string{}
	for _, nic := range ctx.devices.networkMap {
		nameList = append(nameList, nic.DeviceName)
	}

	if len(nameList) == 0 {
		result <- "eth0"
		return
	}

	// The list order was not guaranteed by looping Golang map, so we need to sort it.
	sort.Strings(nameList)

	lastName := nameList[len(nameList)-1]

	var digitsRegexp = regexp.MustCompile(`eth(\d+)`)
	idx := digitsRegexp.FindStringSubmatch(lastName)

	num, err := strconv.Atoi(idx[len(idx)-1])
	if err != nil {
		result <- ""
	}

	result <- fmt.Sprintf("eth%d", num+1)
}

func (ctx *VmContext) allocateInterface(index int, pciAddr int, name string) (*InterfaceCreated, error) {
	var err error
	var inf *network.Settings
	var maps []pod.UserContainerPort

	if index == 0 {
		for _, c := range ctx.userSpec.Containers {
			for _, m := range c.Ports {
				maps = append(maps, m)
			}
		}
	}

	if HDriver.BuildinNetwork() {
		inf, err = ctx.DCtx.AllocateNetwork(ctx.Id, "", maps)
	} else {
		inf, err = network.Allocate(ctx.Id, "", false, maps)
	}

	if err != nil {
		glog.Error("interface creating failed: ", err.Error())

		return &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}, err
	}

	return interfaceGot(index, pciAddr, name, inf)
}

func (ctx *VmContext) ConfigureInterface(index int, pciAddr int, name string, config pod.UserInterface, result chan<- VmEvent) {
	var err error
	var inf *network.Settings
	var maps []pod.UserContainerPort

	if index == 0 {
		for _, c := range ctx.userSpec.Containers {
			for _, m := range c.Ports {
				maps = append(maps, m)
			}
		}
	}

	if HDriver.BuildinNetwork() {
		/* VBox doesn't support join to bridge */
		inf, err = ctx.DCtx.ConfigureNetwork(ctx.Id, "", maps, config)
	} else {
		inf, err = network.Configure(ctx.Id, "", false, maps, config)
	}

	if err != nil {
		glog.Error("interface creating failed: ", err.Error())
		session := &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}
		result <- &DeviceFailed{Session: session}
		return
	}

	session, err := interfaceGot(index, pciAddr, name, inf)
	if err != nil {
		result <- &DeviceFailed{Session: session}
		return
	}

	result <- session
}

func (ctx *VmContext) CreateInterface(index int, pciAddr int, name string) {
	session, err := ctx.allocateInterface(index, pciAddr, name)

	if err != nil {
		ctx.Hub <- &DeviceFailed{Session: session}
		return
	}

	ctx.Hub <- session
}

func (ctx *VmContext) ReleaseInterface(index int, ipAddr string, file *os.File,
	maps []pod.UserContainerPort) {
	var err error
	success := true

	if HDriver.BuildinNetwork() {
		err = ctx.DCtx.ReleaseNetwork(ctx.Id, ipAddr, maps, file)
	} else {
		err = network.Release(ctx.Id, ipAddr, maps, file)
	}

	if err != nil {
		glog.Warning("Unable to release network interface, address: ", ipAddr, err)
		success = false
	}
	ctx.Hub <- &InterfaceReleased{Index: index, Success: success}
}

func interfaceGot(index int, pciAddr int, name string, inf *network.Settings) (*InterfaceCreated, error) {
	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		glog.Error("can not parse cidr")
		return &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name}, err
	}
	var tmp []byte = nw.Mask
	var mask net.IP = tmp

	rt := []*RouteRule{}
	/* Route rule is generated automaticly on first interface,
	 * or generated on the gateway configured interface. */
	if (index == 0 && inf.Automatic) || (!inf.Automatic && inf.Gateway != "") {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	return &InterfaceCreated{
		Index:      index,
		PCIAddr:    pciAddr,
		Bridge:     inf.Bridge,
		HostDevice: inf.Device,
		DeviceName: name,
		Fd:         inf.File,
		MacAddr:    inf.Mac,
		IpAddr:     ip.String(),
		NetMask:    mask.String(),
		RouteTable: rt,
	}, nil
}
