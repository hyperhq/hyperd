package daemon

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/docker/docker/pkg/version"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/servicediscovery"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

type Container struct {
	ApiContainer *apitypes.Container
	CreatedAt    int64
	mountID      string
	fstype       string
	rootfs       string
	initialize   bool
}

type Pod struct {
	Id        string
	CreatedAt int64
	PodStatus *hypervisor.PodStatus
	Spec      *pod.UserPod
	VM        *hypervisor.Vm

	containers []*Container
	volumes    map[string]*hypervisor.VolumeInfo

	transiting chan bool
	sync.RWMutex
}

func convertToRunvContainerSpec(v *apitypes.UserContainer, podTTY bool) pod.UserContainer {
	container := pod.UserContainer{
		Tty:           v.Tty || podTTY,
		Name:          v.Name,
		Image:         v.Image,
		Command:       v.Command,
		Workdir:       v.Workdir,
		Entrypoint:    v.Entrypoint,
		Sysctl:        v.Sysctl,
		RestartPolicy: v.RestartPolicy,
		Labels:        v.Labels,
	}

	if v.User != nil {
		container.User = pod.UserUser{
			Name:             v.User.Name,
			Group:            v.User.Group,
			AdditionalGroups: v.User.AdditionalGroups,
		}
	}

	if len(v.Ports) > 0 {
		ports := make([]pod.UserContainerPort, 0, len(v.Ports))
		for _, p := range v.Ports {
			ports = append(ports, pod.UserContainerPort{
				Protocol:      p.Protocol,
				ContainerPort: int(p.ContainerPort),
				ServicePort:   int(p.ServicePort),
				HostPort:      int(p.HostPort),
			})
		}
		container.Ports = ports
	}

	if len(v.Envs) > 0 {
		envs := make([]pod.UserEnvironmentVar, 0, len(v.Envs))
		for _, env := range v.Envs {
			envs = append(envs, pod.UserEnvironmentVar{
				Env:   env.Env,
				Value: env.Value,
			})
		}
		container.Envs = envs
	}

	if len(v.Volumes) > 0 {
		volumes := make([]pod.UserVolumeReference, 0, len(v.Volumes))
		for _, vol := range v.Volumes {
			volumes = append(volumes, pod.UserVolumeReference{
				Path:     vol.Path,
				ReadOnly: vol.ReadOnly,
				Volume:   vol.Volume,
			})
		}
		container.Volumes = volumes
	}

	if len(v.Files) > 0 {
		files := make([]pod.UserFileReference, 0, len(v.Files))
		for _, f := range v.Files {
			files = append(files, pod.UserFileReference{
				Path:     f.Path,
				Filename: f.Filename,
				Perm:     f.Perm,
				User:     f.User,
				Group:    f.Group,
			})
		}
		container.Files = files
	}

	return container
}

// TODO: remove convertToRunvPodSpec after pod.UserPod is deleted from runv
func convertToRunvPodSpec(podSpec *apitypes.UserPod) (*pod.UserPod, error) {
	var userPod pod.UserPod

	userPod.Name = podSpec.Id
	if podSpec.Id == "" {
		userPod.Name = utils.RandStr(10, "alphanum")
	}

	if podSpec.PortmappingWhiteLists != nil {
		for _, cidr := range podSpec.PortmappingWhiteLists.ExternalNetworks {
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("PortmappingWhiteLists.ExternalNetwork %s format error", cidr)
			}
		}
		filteredInternalNetworks := make([]string, 0)
		for _, cidr := range podSpec.PortmappingWhiteLists.InternalNetworks {
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("PortmappingWhiteLists.InternalNetworks %s format error", cidr)
			}

			// filter cidr out if the cidr is also in ExternalNetworks
			found := false
			for _, ext := range podSpec.PortmappingWhiteLists.ExternalNetworks {
				if cidr == ext {
					found = true
					break
				}
			}
			if !found {
				filteredInternalNetworks = append(filteredInternalNetworks, cidr)
			}
		}

		userPod.PortmappingWhiteLists = &pod.PortmappingWhiteList{
			InternalNetworks: filteredInternalNetworks,
			ExternalNetworks: podSpec.PortmappingWhiteLists.ExternalNetworks,
		}
	}

	userPod.Hostname = podSpec.Hostname
	userPod.Type = podSpec.Type
	userPod.RestartPolicy = podSpec.RestartPolicy
	userPod.Dns = podSpec.Dns
	userPod.Tty = podSpec.Tty
	userPod.Labels = podSpec.Labels

	if podSpec.Labels == nil {
		userPod.Labels = make(map[string]string)
	}

	if podSpec.Resource != nil {
		userPod.Resource = pod.UserResource{
			Vcpu:   int(podSpec.Resource.Vcpu),
			Memory: int(podSpec.Resource.Memory),
		}
	}
	if userPod.Resource.Vcpu == 0 {
		userPod.Resource.Vcpu = 1
	}
	if userPod.Resource.Memory == 0 {
		userPod.Resource.Memory = 128
	}

	if len(podSpec.Containers) > 0 {
		containers := make([]pod.UserContainer, 0, len(podSpec.Containers))
		for _, v := range podSpec.Containers {
			if v.Image == "" {
				return nil, fmt.Errorf("Please specific your image for your container, it can not be null!\n")
			}

			containers = append(containers, convertToRunvContainerSpec(v, userPod.Tty))
		}

		userPod.Containers = containers
	}

	if len(podSpec.Files) > 0 {
		files := make([]pod.UserFile, 0, len(podSpec.Files))
		for _, f := range podSpec.Files {
			files = append(files, pod.UserFile{
				Name:     f.Name,
				Encoding: f.Encoding,
				Uri:      f.Uri,
				Contents: f.Content,
			})
		}
		userPod.Files = files
	}

	if len(podSpec.Volumes) > 0 {
		vols := make([]pod.UserVolume, 0, len(podSpec.Volumes))
		for _, vol := range podSpec.Volumes {
			if vol.Name == "" {
				return nil, fmt.Errorf("Hyper ERROR: please specific your volume name, it can not be null!\n")
			}

			v := pod.UserVolume{
				Name:   vol.Name,
				Driver: vol.Driver,
				Source: vol.Source,
			}
			if vol.Option != nil {
				v.Option = pod.UserVolumeOption{
					Monitors: vol.Option.Monitors,
					Keyring:  vol.Option.Keyring,
					User:     vol.Option.User,
				}
			}
			vols = append(vols, v)
		}
		userPod.Volumes = vols
	}

	if len(podSpec.Services) > 0 {
		services := make([]pod.UserService, 0, len(podSpec.Services))
		for _, svc := range podSpec.Services {
			s := pod.UserService{
				ServiceIP:   svc.ServiceIP,
				ServicePort: int(svc.ServicePort),
				Protocol:    svc.Protocol,
			}
			if len(svc.Hosts) > 0 {
				hosts := make([]pod.UserServiceBackend, 0, len(svc.Hosts))
				for _, host := range svc.Hosts {
					hosts = append(hosts, pod.UserServiceBackend{
						HostIP:   host.HostIP,
						HostPort: int(host.HostPort),
					})
				}
				s.Hosts = hosts
			}
			services = append(services, s)
		}
		userPod.Services = services
	}

	if len(podSpec.Interfaces) > 0 {
		interfaces := make([]pod.UserInterface, 0, len(podSpec.Interfaces))
		for _, i := range podSpec.Interfaces {
			interfaces = append(interfaces, pod.UserInterface{
				Bridge: i.Bridge,
				Ip:     i.Ip,
				Ifname: i.Ifname,
				Mac:    i.Mac,
				Gw:     i.Gateway,
			})
		}
		userPod.Interfaces = interfaces
	}

	if podSpec.Log != nil {
		userPod.LogConfig = pod.PodLogConfig{
			Type:   podSpec.Log.Type,
			Config: podSpec.Log.Config,
		}
	}

	return &userPod, nil
}

func NewPod(podSpec *apitypes.UserPod, id string, data interface{}) (*Pod, error) {
	var err error

	p := &Pod{
		Id:         id,
		CreatedAt:  time.Now().Unix(),
		volumes:    make(map[string]*hypervisor.VolumeInfo),
		transiting: make(chan bool, 1),
	}

	// fill one element in transit chan, only one parallel op is allowed
	p.transiting <- true

	if p.Spec, err = convertToRunvPodSpec(podSpec); err != nil {
		glog.V(1).Infof("Process pod spec failed: %s", err.Error())
		return nil, err
	}

	if err = p.init(data); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Pod) TransitionLock(label string) bool {
	glog.V(1).Infof("lock pod %s for operation %s", p.Id, label)
	select {
	case <-p.transiting:
		glog.V(3).Infof("successfully lock pod %s for operation %s", p.Id, label)
		return true
	default:
		glog.V(3).Infof("failed to lock pod %s for operation %s", p.Id, label)
		return false
	}
}

func (p *Pod) TransitionUnlock(label string) {
	glog.V(1).Infof("unlock pod %s for operation %s", p.Id, label)
	select {
	case p.transiting <- true:
		glog.V(3).Infof("successfully unlock pod %s for operation %s", p.Id, label)
	default:
		glog.V(3).Infof("failed to unlock pod %s for operation %s", p.Id, label)
	}
}

func (p *Pod) GetVM(daemon *Daemon, id string, lazy bool) (err error) {
	if p == nil || p.Spec == nil {
		return errors.New("Pod: unable to create VM without resource info.")
	}
	p.VM, err = daemon.GetVM(id, &p.Spec.Resource, lazy)
	return
}

func (p *Pod) SetVM(id string, vm *hypervisor.Vm) {
	p.PodStatus.Vm = id
	p.VM = vm
}

func (p *Pod) Status() *hypervisor.PodStatus {
	return p.PodStatus
}

func (p *Pod) InitializeFinished(daemon *Daemon) error {
	update := false
	p.Lock()
	defer p.Unlock()

	for idx := range p.Spec.Containers {
		label := fmt.Sprintf("extra.sh.hyper.container.%d.initialize", idx)
		if _, ok := p.Spec.Labels[label]; ok {
			p.containers[idx].initialize = false
			delete(p.Spec.Labels, label)
			update = true
		}
	}

	if update == false {
		return nil
	}

	spec, err := json.Marshal(p.Spec)
	if err != nil {
		return err
	}

	if err := daemon.db.UpdatePod(p.Id, spec); err != nil {
		return err
	}

	return nil
}

func (p *Pod) DoCreate(daemon *Daemon) error {
	jsons, err := p.TryLoadContainers(daemon)
	if err != nil {
		return err
	}

	if err = p.createNewContainers(daemon, jsons); err != nil {
		return err
	}

	if err = p.ParseContainerJsons(daemon, jsons); err != nil {
		return err
	}

	if err = p.CreateVolumes(daemon); err != nil {
		return err
	}

	if err = p.UpdateContainerStatus(jsons); err != nil {
		return err
	}

	return nil
}

func (p *Pod) init(data interface{}) error {
	if err := p.Spec.Validate(); err != nil {
		return err
	}

	if err := p.preprocess(); err != nil {
		return err
	}

	p.PodStatus = hypervisor.NewPod(p.Id, p.Spec, &hypervisor.HandleEvent{hyperHandlePodEvent, data})

	return nil
}

func (p *Pod) preprocess() (err error) {
	if p.Spec == nil {
		return fmt.Errorf("No spec available for preprocess: %s", p.Id)
	}

	if err = ParseServiceDiscovery(p.Id, p.Spec); err != nil {
		return err
	}

	if err = p.setupServices(); err != nil {
		return err
	}

	if err = p.setupEtcHosts(); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			glog.Infof("preprocess pod %s failed", p.Id)
			p.cleanupEtcHosts()
		}
	}()

	if err = p.setupDNS(); err != nil {
		glog.Warning("Fail to prepare DNS for %s: %v", p.Id, err)
		return err
	}

	return nil
}

func (p *Pod) TryLoadContainers(daemon *Daemon) ([]*dockertypes.ContainerJSON, error) {
	var (
		containerJsons = make([]*dockertypes.ContainerJSON, len(p.Spec.Containers))
		rsp            *dockertypes.ContainerJSON
		ok             bool
	)

	if ids, err := daemon.db.GetP2C(p.Id); ids != nil {
		glog.V(3).Infof("loaded containers for pod %s: %v", p.Id, ids)

		containerNames := make(map[string]int)

		for idx, c := range p.Spec.Containers {
			containerNames[c.Name] = idx
		}
		for _, id := range ids {
			glog.V(3).Infof("Loading container %s of pod %s", id, p.Id)
			if r, err := daemon.ContainerInspect(id, false, version.Version("1.21")); err == nil {
				rsp, ok = r.(*dockertypes.ContainerJSON)
				if !ok {
					if glog.V(1) {
						glog.Warningf("fail to load container %s for pod %s", id, p.Id)
					}
					continue
				}

				n := strings.TrimLeft(rsp.Name, "/")
				if idx, ok := containerNames[n]; ok {
					glog.V(1).Infof("Found exist container %s (%s), pod: %s", n, id, p.Id)
					containerJsons[idx] = rsp
				} else if glog.V(1) {
					glog.Warningf("loaded container %s (%s) is not belongs to pod %s", n, id, p.Id)
				}
			}
		}
	} else {
		glog.V(3).Infof("no containers for pod %s loaded: %v", p.Id, err)
	}

	return containerJsons, nil
}

func (p *Pod) createNewContainers(daemon *Daemon, jsons []*dockertypes.ContainerJSON) error {

	var (
		ok  bool
		err error
		ccs dockertypes.ContainerCreateResponse
		rsp *dockertypes.ContainerJSON
		r   interface{}

		cleanup = func(id string) {
			if err != nil {
				glog.V(1).Infof("rollback container %s of %s", id, p.Id)
				daemon.Daemon.ContainerRm(id, &dockertypes.ContainerRmConfig{})
			}
		}
	)

	for idx, c := range p.Spec.Containers {
		if jsons[idx] != nil {
			glog.V(1).Infof("do not need to create container %s of pod %s[%d]", c.Name, p.Id, idx)
			continue
		}

		config := &container.Config{
			Image:           c.Image,
			Cmd:             strslice.New(c.Command...),
			NetworkDisabled: true,
		}

		if len(c.Entrypoint) != 0 {
			config.Entrypoint = strslice.New(c.Entrypoint...)
		}

		if len(c.Envs) != 0 {
			envs := []string{}
			for _, env := range c.Envs {
				envs = append(envs, env.Env+"="+env.Value)
			}
			config.Env = envs
		}

		ccs, err = daemon.Daemon.ContainerCreate(dockertypes.ContainerCreateConfig{
			Name:   c.Name,
			Config: config,
		})

		if err != nil {
			glog.Error(err.Error())
			return err
		}
		defer cleanup(ccs.ID)

		// Set Initialize to true, need to initialize container environment
		label := fmt.Sprintf("extra.sh.hyper.container.%d.initialize", idx)
		p.Spec.Labels[label] = "yes"

		glog.Infof("create container %s", ccs.ID)
		if r, err = daemon.ContainerInspect(ccs.ID, false, version.Version("1.21")); err != nil {
			return err
		}

		if rsp, ok = r.(*dockertypes.ContainerJSON); !ok {
			err = fmt.Errorf("fail to unpack container json response for %s of %s", c.Name, p.Id)
			return err
		}

		jsons[idx] = rsp
	}

	return nil
}

func (p *Pod) ParseContainerJsons(daemon *Daemon, jsons []*dockertypes.ContainerJSON) (err error) {
	err = nil
	p.containers = []*Container{}

	for i, c := range p.Spec.Containers {
		if jsons[i] == nil {
			estr := fmt.Sprintf("container %s of pod %s does not have inspect json", c.Name, p.Id)
			glog.Error(estr)
			return errors.New(estr)
		}

		var (
			info *dockertypes.ContainerJSON = jsons[i]
			ci   *Container                 = &Container{
				ApiContainer: &apitypes.Container{},
			}
		)

		if c.Name == "" {
			p.Spec.Containers[i].Name = strings.TrimLeft(info.Name, "/")
		}
		if c.Image == "" {
			p.Spec.Containers[i].Image = info.Config.Image
		}
		glog.Infof("container name %s, image %s", c.Name, c.Image)

		mountId, err := GetMountIdByContainer(daemon.Storage.Type(), info.ID)
		if err != nil {
			estr := fmt.Sprintf("Cannot find mountID for container %s : %s", info.ID, err)
			glog.Error(estr)
			return errors.New(estr)
		}

		ci.ApiContainer.ContainerID = info.ID
		ci.ApiContainer.User = info.Config.User
		ci.mountID = mountId
		ci.ApiContainer.WorkingDir = info.Config.WorkingDir
		ci.ApiContainer.Commands = []string{info.Path}
		ci.ApiContainer.Args = info.Args

		created, err := utils.ParseTimeString(info.Created)
		if err != nil {
			estr := fmt.Sprintf("Parse container created time failed: %v", err)
			glog.Error(estr)
			return errors.New(estr)
		}
		ci.CreatedAt = created

		// We should ignore these two in runv, instead of clear them, but here is a work around
		p.Spec.Containers[i].Entrypoint = []string{}
		p.Spec.Containers[i].Command = []string{}
		glog.Infof("container info config %v, Cmd %v, Args %v", info.Config, info.Config.Cmd.Slice(), info.Args)

		var envs []*apitypes.EnvironmentVar
		for _, v := range info.Config.Env {
			pair := strings.SplitN(v, "=", 2)
			if len(pair) == 2 && pair[1] != "" {
				envs = append(envs, &apitypes.EnvironmentVar{
					Env:   pair[0],
					Value: pair[1],
				})
			}
		}
		ci.ApiContainer.Env = envs

		label := fmt.Sprintf("extra.sh.hyper.container.%d.initialize", i)
		if value, ok := p.Spec.Labels[label]; ok {
			if value == "true" || value == "yes" || value == "1" {
				ci.initialize = true
			}
		}

		p.processImageVolumes(info, info.ID, &p.Spec.Containers[i])

		p.containers = append(p.containers, ci)
		glog.V(1).Infof("Container Info is \n%v", ci)
	}

	return nil
}

func GetMountIdByContainer(driver, cid string) (string, error) {
	idPath := path.Join(utils.HYPER_ROOT, fmt.Sprintf("image/%s/layerdb/mounts/%s/mount-id", driver, cid))
	if _, err := os.Stat(idPath); err != nil && os.IsNotExist(err) {
		return "", err
	}

	id, err := ioutil.ReadFile(idPath)
	if err != nil {
		return "", err
	}

	return string(id), nil
}

func (p *Pod) CreateVolumes(daemon *Daemon) error {

	var (
		vol *hypervisor.VolumeInfo
		err error
	)

	sd := daemon.Storage
	for i := range p.Spec.Volumes {
		if p.Spec.Volumes[i].Source == "" {
			vol, err = sd.CreateVolume(daemon, p.Id, p.Spec.Volumes[i].Name)
			if err != nil {
				return err
			}

			p.Spec.Volumes[i].Source = vol.Filepath
			if sd.Type() != "devicemapper" {
				p.Spec.Volumes[i].Driver = "vfs"
			} else {
				// type other than doesn't need to be mounted
				p.Spec.Volumes[i].Driver = "raw"
			}
		}
	}
	return nil
}

func (p *Pod) UpdateContainerStatus(jsons []*dockertypes.ContainerJSON) error {
	p.PodStatus.Containers = []*hypervisor.ContainerStatus{}
	for idx, c := range p.Spec.Containers {
		if jsons[idx] == nil {
			estr := fmt.Sprintf("container %s of pod %s does not have inspect json", c.Name, p.Id)
			glog.Error(estr)
			return errors.New(estr)
		}

		cmds := append([]string{jsons[idx].Path}, jsons[idx].Args...)
		p.PodStatus.AddContainer(jsons[idx].ID, "/"+c.Name, jsons[idx].Image, cmds, types.S_POD_CREATED)
	}
	return nil
}

func processInjectFiles(container *pod.UserContainer, files map[string]pod.UserFile, sd Storage,
	id, rootPath, sharedDir string) error {
	for _, f := range container.Files {
		targetPath := f.Path
		if strings.HasSuffix(targetPath, "/") {
			targetPath = targetPath + f.Filename
		}
		file, ok := files[f.Filename]
		if !ok {
			continue
		}

		var src io.Reader

		if file.Uri != "" {
			urisrc, err := utils.UriReader(file.Uri)
			if err != nil {
				return err
			}
			defer urisrc.Close()
			src = urisrc
		} else {
			src = strings.NewReader(file.Contents)
		}

		switch file.Encoding {
		case "base64":
			src = base64.NewDecoder(base64.StdEncoding, src)
		default:
		}

		err := sd.InjectFile(src, id, targetPath, sharedDir,
			utils.PermInt(f.Perm), utils.UidInt(f.User), utils.UidInt(f.Group))
		if err != nil {
			glog.Error("got error when inject files ", err.Error())
			return err
		}
	}

	return nil
}

func (p *Pod) processImageVolumes(config *dockertypes.ContainerJSON, id string, container *pod.UserContainer) {
	if config.Config.Volumes == nil {
		return
	}

	userPod := p.Spec

	existed := make(map[string]bool)
	for _, v := range container.Volumes {
		existed[v.Path] = true
	}

	for tgt := range config.Config.Volumes {
		if _, ok := existed[tgt]; ok {
			continue
		}

		n := id + strings.Replace(tgt, "/", "_", -1)
		v := pod.UserVolume{
			Name:   n,
			Source: "",
		}
		r := pod.UserVolumeReference{
			Volume:   n,
			Path:     tgt,
			ReadOnly: false,
		}

		p.volumes[n] = &hypervisor.VolumeInfo{Name: n, DockerVolume: true}
		userPod.Volumes = append(userPod.Volumes, v)
		container.Volumes = append(container.Volumes, r)
	}
}

func (p *Pod) setupServices() error {

	err := servicediscovery.PrepareServices(p.Spec, p.Id)
	if err != nil {
		glog.Errorf("PrepareServices failed %s", err.Error())
	}
	return err
}

// SetupEtcHosts sets /etc/hosts for each container
func (p *Pod) setupEtcHosts() (err error) {
	var (
		hostsVolumeName = "etchosts-volume"
		hostVolumePath  = ""
		hostsPath       = "/etc/hosts"
	)

	if p.Spec == nil {
		return
	}

	for idx, c := range p.Spec.Containers {
		insert := true

		for _, v := range c.Volumes {
			if v.Path == hostsPath {
				insert = false
				break
			}
		}

		for _, f := range c.Files {
			if f.Path == hostsPath {
				insert = false
				break
			}
		}

		if !insert {
			continue
		}

		if hostVolumePath == "" {
			hostVolumePath, err = prepareHosts(p.Id)
			if err != nil {
				return
			}

			p.Spec.Volumes = append(p.Spec.Volumes, pod.UserVolume{
				Name:   hostsVolumeName,
				Source: hostVolumePath,
				Driver: "vfs",
			})
		}

		p.Spec.Containers[idx].Volumes = append(c.Volumes, pod.UserVolumeReference{
			Path:     hostsPath,
			Volume:   hostsVolumeName,
			ReadOnly: false,
		})
	}

	return
}

func (p *Pod) cleanupEtcHosts() {
	glog.Infof("cleanupEtcHost for %s", p.Id)
	if p.Spec == nil {
		glog.Infof("p.Spec is nil")
		return
	}

	if err := cleanupHosts(p.Id); err != nil {
		glog.V(1).Infof("cleanup hosts for pod %s failed, %s", p.Id, err.Error())
	}

	return
}

/***
  PrepareDNS() Set the resolv.conf of host to each container, except the following cases:

  - if the pod has a `dns` field with values, the pod will follow the dns setup, and daemon
    won't insert resolv.conf file into any containers
  - if the pod has a `file` which source is uri "file:///etc/resolv.conf", this mean the user
    will handle this file by himself/herself, daemon won't touch the dns setting even if the file
    is not referenced by any containers. This could be a method to prevent the daemon from unwanted
    setting the dns configuration
  - if a container has a file config in the pod spec with `/etc/resolv.conf` as target `path`,
    then this container won't be set as the file from hosts. Then a user can specify the content
    of the file.

*/
func (p *Pod) setupDNS() (err error) {
	err = nil
	var (
		resolvconf = "/etc/resolv.conf"
		fileId     = p.Id + "-resolvconf"
	)

	if p.Spec == nil {
		estr := "No Spec available for insert a DNS configuration"
		glog.V(1).Info(estr)
		err = fmt.Errorf(estr)
		return
	}

	if len(p.Spec.Dns) > 0 {
		glog.V(1).Info("Already has DNS config, bypass DNS insert")
		return
	}

	if stat, e := os.Stat(resolvconf); e != nil || !stat.Mode().IsRegular() {
		glog.V(1).Info("Host resolv.conf does not exist or not a regular file, do not insert DNS conf")
		return
	}

	for _, src := range p.Spec.Files {
		if src.Uri == "file:///etc/resolv.conf" {
			glog.V(1).Info("Already has resolv.conf configured, bypass DNS insert")
			return
		}
	}

	p.Spec.Files = append(p.Spec.Files, pod.UserFile{
		Name:     fileId,
		Encoding: "raw",
		Uri:      "file://" + resolvconf,
	})

	for idx, c := range p.Spec.Containers {
		insert := true

		for _, f := range c.Files {
			if f.Path == resolvconf {
				insert = false
				break
			}
		}

		if !insert {
			continue
		}

		p.Spec.Containers[idx].Files = append(c.Files, pod.UserFileReference{
			Path:     resolvconf,
			Filename: fileId,
			Perm:     "0644",
		})
	}

	return
}

func (p *Pod) setupMountsAndFiles(sd Storage) (err error) {
	if len(p.containers) != len(p.Spec.Containers) {
		estr := fmt.Sprintf("Prepare error, pod %s does not get container infos well", p.Id)
		glog.Error(estr)
		err = errors.New(estr)
		return err
	}

	var (
		sharedDir = path.Join(hypervisor.BaseDir, p.VM.Id, hypervisor.ShareDirTag)
		files     = make(map[string](pod.UserFile))
	)

	for _, f := range p.Spec.Files {
		files[f.Name] = f
	}

	for i, c := range p.PodStatus.Containers {
		ci := p.containers[i]

		glog.Infof("container ID: %s, mountId %s\n", c.Id, ci.mountID)
		err = sd.PrepareContainer(ci, sharedDir)
		if err != nil {
			return err
		}

		err = processInjectFiles(&p.Spec.Containers[i], files, sd, ci.mountID, sd.RootPath(), sharedDir)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Pod) mountVolumes(daemon *Daemon, sd Storage) (err error) {
	err = nil

	var (
		sharedDir = path.Join(hypervisor.BaseDir, p.VM.Id, hypervisor.ShareDirTag)
	)

	for _, v := range p.Spec.Volumes {
		var volInfo *hypervisor.VolumeInfo
		if v.Source == "" {
			err = fmt.Errorf("volume %s in pod %s is not created", v.Name, p.Id)
			return err
		}

		volInfo, err = ProbeExistingVolume(&v, sharedDir)
		if err != nil {
			return err
		}

		if vol, ok := p.volumes[v.Name]; ok {
			vol.Filepath = volInfo.Filepath
			vol.Fstype = volInfo.Fstype
			vol.Format = volInfo.Format
			continue
		}

		p.volumes[v.Name] = volInfo
	}

	return nil
}

func (p *Pod) prepareEtcHosts() error {
	var (
		hostsVolumeName = "etchosts-volume"
		hostVolumePath  = path.Join(utils.HYPER_ROOT, "hosts", p.Id, defaultHostsFilename)
	)

	for _, v := range p.Spec.Volumes {
		// FIXME: check if the user configure the hosts volume
		if v.Name == hostsVolumeName && v.Source == hostVolumePath {
			_, err := prepareHosts(p.Id)
			return err
		}
	}

	return nil
}

func (p *Pod) Prepare(daemon *Daemon) (err error) {
	if err = p.prepareEtcHosts(); err != nil {
		return
	}

	if err = p.setupMountsAndFiles(daemon.Storage); err != nil {
		return
	}

	if err = p.mountVolumes(daemon, daemon.Storage); err != nil {
		return
	}

	return nil
}

func (p *Pod) cleanupMountsAndFiles(sd Storage, sharedDir string) {
	for i := range p.PodStatus.Containers {
		mountId := p.containers[i].mountID
		sd.CleanupContainer(mountId, sharedDir)
	}
}

func (p *Pod) cleanupVolumes(sd Storage, sharedDir string) {
	for _, v := range p.volumes {
		CleanupExistingVolume(v.Fstype, v.Filepath, sharedDir)
	}
}

func (p *Pod) Cleanup(daemon *Daemon) {
	p.PodStatus.Vm = ""

	if p.VM != nil {
		sharedDir := path.Join(hypervisor.BaseDir, p.VM.Id, hypervisor.ShareDirTag)
		vmDir := path.Join(hypervisor.BaseDir, p.VM.Id)
		p.cleanupVolumes(daemon.Storage, sharedDir)
		p.cleanupMountsAndFiles(daemon.Storage, sharedDir)
		os.RemoveAll(vmDir)

		p.VM = nil
	}

	daemon.db.DeleteVMByPod(p.Id)

	p.cleanupEtcHosts()

	p.PodStatus.CleanupExec()

	if p.PodStatus.Status == types.S_POD_NONE {
		daemon.RemovePodResource(p)
	}
}

func stopLogger(mypod *hypervisor.PodStatus) {
	for _, c := range mypod.Containers {
		if c.Logs.Driver == nil {
			continue
		}

		c.Logs.Driver.Close()
	}
}

func (p *Pod) getLogger(daemon *Daemon) (err error) {
	if p.Spec.LogConfig.Type == "" {
		p.Spec.LogConfig.Type = daemon.DefaultLog.Type
		p.Spec.LogConfig.Config = daemon.DefaultLog.Config
	}

	if p.Spec.LogConfig.Type == "none" {
		return nil
	}

	var (
		needLogger []int = []int{}
		creator    logger.Creator
	)

	for i, c := range p.PodStatus.Containers {
		if c.Logs.Driver == nil {
			needLogger = append(needLogger, i)
		}
	}

	if len(needLogger) == 0 && p.PodStatus.Status == types.S_POD_RUNNING {
		return nil
	}

	if err = logger.ValidateLogOpts(p.Spec.LogConfig.Type, p.Spec.LogConfig.Config); err != nil {
		return
	}
	creator, err = logger.GetLogDriver(p.Spec.LogConfig.Type)
	if err != nil {
		return
	}
	glog.V(1).Infof("configuring log driver [%s] for %s", p.Spec.LogConfig.Type, p.Id)

	prefix := daemon.DefaultLog.PathPrefix
	if daemon.DefaultLog.PodIdInPath {
		prefix = filepath.Join(prefix, p.Id)
	}
	if err = os.MkdirAll(prefix, os.FileMode(0755)); err != nil {
		glog.Error("cannot create container log dir ", prefix)
		return
	}

	for i, c := range p.PodStatus.Containers {
		ctx := logger.Context{
			Config:             p.Spec.LogConfig.Config,
			ContainerID:        c.Id,
			ContainerName:      c.Name,
			ContainerImageName: p.Spec.Containers[i].Image,
			ContainerCreated:   time.Now(), //FIXME: should record creation time in PodStatus
		}

		if p.containers != nil && len(p.containers) > i {
			ctx.ContainerEntrypoint = p.containers[i].ApiContainer.WorkingDir
			ctx.ContainerArgs = p.containers[i].ApiContainer.Commands
			ctx.ContainerImageID = p.containers[i].ApiContainer.Image
		}

		if p.Spec.LogConfig.Type == jsonfilelog.Name {
			ctx.LogPath = filepath.Join(prefix, fmt.Sprintf("%s-json.log", c.Id))
			glog.V(1).Info("configure container log to ", ctx.LogPath)
		}

		if c.Logs.Driver, err = creator(ctx); err != nil {
			return
		}
		glog.V(1).Infof("configured logger for %s/%s (%s)", p.Id, c.Id, c.Name)
	}

	return nil
}

func (p *Pod) startLogging(daemon *Daemon) (err error) {
	err = nil

	if err = p.getLogger(daemon); err != nil {
		return
	}

	if p.Spec.LogConfig.Type == "none" {
		return nil
	}

	for _, c := range p.PodStatus.Containers {
		var stdout, stderr io.Reader

		if stdout, stderr, err = p.VM.GetLogOutput(c.Id, nil); err != nil {
			return
		}
		c.Logs.Copier = logger.NewCopier(c.Id, map[string]io.Reader{"stdout": stdout, "stderr": stderr}, c.Logs.Driver)
		c.Logs.Copier.Run()

		if jl, ok := c.Logs.Driver.(*jsonfilelog.JSONFileLogger); ok {
			c.Logs.LogPath = jl.LogPath()
		}
	}

	return nil
}

func (p *Pod) AttachTtys(daemon *Daemon, streams []*hypervisor.TtyIO) (err error) {

	ttyContainers := p.containers
	if p.Spec.Type == "service-discovery" {
		ttyContainers = p.containers[1:]
	}

	for idx, str := range streams {
		if idx >= len(ttyContainers) {
			break
		}

		containerId := ttyContainers[idx].ApiContainer.ContainerID
		err = p.VM.Attach(str, containerId, nil)
		if err != nil {
			glog.Errorf("Failed to attach to container %s before start pod", containerId)
			return
		}
		glog.V(1).Infof("Attach to container %s before start pod", containerId)
	}

	return nil
}

func convertToRunvContainerInfo(list []*Container) []*hypervisor.ContainerInfo {
	results := make([]*hypervisor.ContainerInfo, len(list))

	for idx, c := range list {
		envs := make(map[string]string)
		for _, env := range c.ApiContainer.Env {
			envs[env.Env] = env.Value
		}
		results[idx] = &hypervisor.ContainerInfo{
			Id:         c.ApiContainer.ContainerID,
			User:       c.ApiContainer.User,
			MountId:    c.mountID,
			Rootfs:     c.rootfs,
			Fstype:     c.fstype,
			Workdir:    c.ApiContainer.WorkingDir,
			Cmd:        append(c.ApiContainer.Commands, c.ApiContainer.Args...),
			Initialize: c.initialize,
			Envs:       envs,
			Image:      pod.UserVolume{Source: c.ApiContainer.Image},
		}
	}

	return results
}

func (p *Pod) Start(daemon *Daemon, vmId string, lazy bool, streams []*hypervisor.TtyIO) (*types.VmResponse, error) {

	var (
		err       error = nil
		preparing bool  = true
	)

	if p.PodStatus.Status == types.S_POD_RUNNING ||
		(p.PodStatus.Type == "kubernetes" && p.PodStatus.Status != types.S_POD_CREATED) {
		estr := fmt.Sprintf("invalid pod status for start %v", p.PodStatus.Status)
		glog.Warning(estr)
		return nil, errors.New(estr)
	}

	if err = p.GetVM(daemon, vmId, lazy); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil && preparing {
			id := p.VM.Id
			p.Lock()
			p.Cleanup(daemon)
			p.Unlock()

			if vmId == "" {
				daemon.KillVm(id)
			}
		}
	}()

	if err = p.Prepare(daemon); err != nil {
		return nil, err
	}

	if err = p.startLogging(daemon); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			stopLogger(p.PodStatus)
		}
	}()

	if err = p.AttachTtys(daemon, streams); err != nil {
		return nil, err
	}

	// now start, the pod handler will deal with the vm
	preparing = false

	vmResponse := p.VM.StartPod(p.PodStatus, p.Spec, convertToRunvContainerInfo(p.containers), p.volumes)
	if vmResponse.Data == nil {
		err = fmt.Errorf("VM %s start failed with code %d: %s", vmResponse.VmId, vmResponse.Code, vmResponse.Cause)
		return vmResponse, err
	}

	err = daemon.db.UpdateVM(p.VM.Id, vmResponse.Data.([]byte))
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}
	// add or update the Vm info for POD
	glog.V(1).Infof("Add or Update the VM info for pod(%s)", p.Id)
	err = daemon.db.UpdateP2V(p.Id, p.VM.Id)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	return vmResponse, nil
}

func hyperHandlePodEvent(vmResponse *types.VmResponse, data interface{},
	mypod *hypervisor.PodStatus, vm *hypervisor.Vm) bool {
	daemon := data.(*Daemon)

	if vmResponse.Code == types.E_VM_SHUTDOWN { // vm exited, sucessful or not
		stopLogger(mypod)
		daemon.PodStopped(mypod.Id)

		if mypod.Type == "kubernetes" {
			cleanup := false
			switch mypod.Status {
			case types.S_POD_SUCCEEDED:
				if mypod.RestartPolicy == "always" {
					daemon.RestartPod(mypod)
					break
				}
				cleanup = true
			case types.S_POD_FAILED:
				if mypod.RestartPolicy != "never" {
					daemon.RestartPod(mypod)
					break
				}
				cleanup = true
			default:
				break
			}
			if cleanup {
				pod, ok := daemon.PodList.Get(mypod.Id)
				if ok {
					daemon.RemovePodContainer(pod)
				}
				daemon.DeleteVolumeId(mypod.Id)
			}
		}
		return true
	}

	return false
}
