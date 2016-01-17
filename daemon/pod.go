package daemon

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/servicediscovery"
	"github.com/hyperhq/hyper/storage"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CmdPodCreate(job *engine.Job) (err error) {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}
	podArgs := job.Args[0]
	autoRemove := false
	if job.Args[1] == "yes" || job.Args[1] == "true" {
		autoRemove = true
	}
	prefetchVm := false
	if job.Args[2] == "yes" || job.Args[2] == "true" {
		prefetchVm = true
	}

	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	glog.V(1).Infof("podArgs: %s", podArgs)

	spec, err := ProcessPodBytes([]byte(podArgs), podId)
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return err
	}

	if err = spec.Validate(); err != nil {
		return err
	}

	var prefetchedVm PrefetchedVm
	if prefetchVm {
		prefetchedVm, err := daemon.prefetchVm(spec)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				prefetchedVm.cancel(daemon)
			}
		}()
	}

	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()

	pod, err := CreatePod(daemon, daemon.DockerCli, podId, podArgs, spec, autoRemove)
	if err != nil {
		return err
	}
	pod.prefetchedVm = prefetchedVm

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

// CmdPodLabels updates labels of the specified pod
func (daemon *Daemon) CmdPodLabels(job *engine.Job) error {
	override := false
	if job.Args[1] == "true" || job.Args[1] == "yes" {
		override = true
	}

	labels := make(map[string]string)
	if len(job.Args[2]) == 0 {
		return fmt.Errorf("labels can't be null")
	}
	if err := json.Unmarshal([]byte(job.Args[2]), &labels); err != nil {
		return err
	}

	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodList.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")

	var (
		pod *Pod
		ok  bool
	)
	if strings.Contains(job.Args[0], "pod-") {
		podId := job.Args[0]
		pod, ok = daemon.PodList.Get(podId)
		if !ok {
			return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
		}
	} else {
		pod = daemon.PodList.GetByName(job.Args[0])
		if pod == nil {
			return fmt.Errorf("Can not get Pod info with pod name(%s)", job.Args[0])
		}
	}

	if pod.spec.Labels == nil {
		pod.spec.Labels = make(map[string]string)
	}

	for k := range labels {
		if _, ok := pod.spec.Labels[k]; ok && !override {
			return fmt.Errorf("Can't update label %s without override", k)
		}
	}

	for k, v := range labels {
		pod.spec.Labels[k] = v
	}

	spec, err := json.Marshal(pod.spec)
	if err != nil {
		return err
	}

	if err := daemon.WritePodToDB(pod.id, spec); err != nil {
		return err
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", pod.id)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdPodStart(job *engine.Job) error {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	var (
		tag         string              = ""
		ttys        []*hypervisor.TtyIO = []*hypervisor.TtyIO{}
		ttyCallback chan *types.VmResponse
	)

	podId := job.Args[0]
	vmId := job.Args[1]
	if len(job.Args) > 2 {
		tag = job.Args[2]
	}
	if tag != "" {
		glog.V(1).Info("Pod Run with client terminal tag: ", tag)
		ttyCallback = make(chan *types.VmResponse, 1)
		ttys = append(ttys, &hypervisor.TtyIO{
			Stdin:     job.Stdin,
			Stdout:    job.Stdout,
			ClientTag: tag,
			Callback:  ttyCallback,
		})
	}

	glog.Infof("pod:%s, vm:%s", podId, vmId)
	// Do the status check for the given pod
	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	if _, ok := daemon.PodList.Get(podId); !ok {
		glog.V(2).Infof("unlock PodList")
		daemon.PodList.Unlock()
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode() && vmId == ""

	code, cause, err := daemon.StartPod(podId, "", vmId, nil, lazy, false, types.VM_KEEP_NONE, ttys)
	if err != nil {
		glog.Error(err.Error())
		glog.V(2).Infof("unlock PodList")
		daemon.PodList.Unlock()
		return err
	}

	if len(ttys) > 0 {
		glog.V(2).Infof("unlock PodList")
		daemon.PodList.Unlock()
		daemon.GetExitCode(podId, tag, ttyCallback)
		return nil
	}
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

type PrefetchedVm struct {
	vm *hypervisor.Vm
}

// dummy implementation of prefetchVm
func (daemon *Daemon) prefetchVm(spec *pod.UserPod) (PrefetchedVm, error) {
	glog.V(2).Info("prefetchVm")
	vm, err := daemon.StartVm("", spec.Resource.Vcpu, spec.Resource.Memory, false, 0)
	return PrefetchedVm{vm: vm}, err
}

func (p *PrefetchedVm) cancel(daemon *Daemon) {
	if p.vm != nil {
		glog.V(2).Info("cancel PrefetchedVm")
		daemon.KillVm(p.vm.Id)
	}
	p.vm = nil
}

func (p *PrefetchedVm) get() *hypervisor.Vm {
	glog.V(2).Info("get PrefetchedVm")
	vm := p.vm
	p.vm = nil
	return vm
}

// I'd like to move the remain part of this file to another file.
type Pod struct {
	id           string
	status       *hypervisor.PodStatus
	spec         *pod.UserPod
	vm           *hypervisor.Vm
	prefetchedVm PrefetchedVm
	containers   []*hypervisor.ContainerInfo
	volumes      []*hypervisor.VolumeInfo
}

func (p *Pod) GetVM(daemon *Daemon, id string, lazy bool, keep int) (err error) {
	if p == nil || p.spec == nil {
		return errors.New("Pod: unable to create VM without resource info.")
	}
	if id == "" {
		p.vm = p.prefetchedVm.get()
		if p.vm != nil {
			return
		}
	}
	p.vm, err = daemon.GetVM(id, &p.spec.Resource, lazy, keep)
	return
}

func (p *Pod) SetVM(id string, vm *hypervisor.Vm) {
	p.status.Vm = id
	p.vm = vm
}

func (p *Pod) KillVM(daemon *Daemon) {
	if p.vm != nil {
		daemon.KillVm(p.vm.Id)
		p.vm = nil
	}
}

func (p *Pod) Status() *hypervisor.PodStatus {
	return p.status
}

func CreatePod(daemon *Daemon, dclient DockerInterface, podId, podArgs string, spec *pod.UserPod, autoremove bool) (*Pod, error) {
	resPath := filepath.Join(DefaultResourcePath, podId)
	if err := os.MkdirAll(resPath, os.FileMode(0755)); err != nil {
		glog.Error("cannot create resource dir ", resPath)
		return nil, err
	}

	status := hypervisor.NewPod(podId, spec)
	status.Handler.Handle = hyperHandlePodEvent
	status.Autoremove = autoremove
	status.ResourcePath = resPath

	pod := &Pod{
		id:     podId,
		status: status,
		spec:   spec,
	}

	if err := pod.InitContainers(daemon, dclient); err != nil {
		return nil, err
	}

	if err := daemon.AddPod(pod, podArgs); err != nil {
		return nil, err
	}

	return pod, nil
}

func (p *Pod) InitContainers(daemon *Daemon, dclient DockerInterface) (err error) {

	type cinfo struct {
		id    string
		name  string
		image string
	}

	var (
		containers map[string]*cinfo = make(map[string]*cinfo)
		created    []string          = []string{}
	)

	// trying load existing containers from db
	if ids, _ := daemon.GetPodContainersByName(p.id); ids != nil {
		for _, id := range ids {
			if jsonResponse, err := daemon.DockerCli.GetContainerInfo(id); err == nil {
				n := strings.TrimLeft(jsonResponse.Name, "/")
				containers[n] = &cinfo{
					id:    id,
					name:  jsonResponse.Name,
					image: jsonResponse.Config.Image,
				}
				glog.V(1).Infof("Found exist container %s (%s), image: %s", n, id, jsonResponse.Config.Image)
			}
		}
	}

	defer func() {
		if err != nil {
			for _, cid := range created {
				dclient.SendCmdDelete(cid)
			}
		}
	}()

	glog.V(1).Info("Process the Containers section in POD SPEC")
	for _, c := range p.spec.Containers {

		glog.V(1).Info("trying to init container ", c.Name)

		if info, ok := containers[c.Name]; ok {
			p.status.AddContainer(info.id, info.name, info.image, []string{}, types.S_POD_CREATED)
			continue
		}

		var (
			cId []byte
			rsp *dockertypes.ContainerJSON
		)

		cId, _, err = dclient.SendCmdCreate(c.Name, c.Image, []string{}, nil)
		if err != nil {
			glog.Error(err.Error())
			return
		}

		created = append(created, string(cId))

		if rsp, err = dclient.GetContainerInfo(string(cId)); err != nil {
			return
		}

		p.status.AddContainer(string(cId), rsp.Name, rsp.Config.Image, []string{}, types.S_POD_CREATED)
	}

	return nil
}

//FIXME: there was a `config` argument passed by docker/builder, but we never processed it.
func (daemon *Daemon) CreatePod(podId, podArgs string, autoremove bool) (*Pod, error) {
	glog.V(1).Infof("podArgs: %s", podArgs)

	spec, err := ProcessPodBytes([]byte(podArgs), podId)
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return nil, err
	}

	if err = spec.Validate(); err != nil {
		return nil, err
	}

	return CreatePod(daemon, daemon.DockerCli, podId, podArgs, spec, autoremove)
}

func (p *Pod) PrepareContainers(sd Storage, dclient DockerInterface) (err error) {
	err = nil
	p.containers = []*hypervisor.ContainerInfo{}

	var (
		sharedDir = path.Join(hypervisor.BaseDir, p.vm.Id, hypervisor.ShareDirTag)
	)

	files := make(map[string](pod.UserFile))
	for _, f := range p.spec.Files {
		files[f.Name] = f
	}

	for i, c := range p.status.Containers {
		var (
			info *dockertypes.ContainerJSON
			ci   *hypervisor.ContainerInfo
		)
		info, err = getContinerInfo(dclient, c)

		ci, err = sd.PrepareContainer(c.Id, sharedDir)
		if err != nil {
			return err
		}
		ci.Workdir = info.Config.WorkingDir
		ci.Entrypoint = info.Config.Entrypoint.Slice()
		ci.Cmd = info.Config.Cmd.Slice()

		env := make(map[string]string)
		for _, v := range info.Config.Env {
			env[v[:strings.Index(v, "=")]] = v[strings.Index(v, "=")+1:]
		}
		for _, e := range p.spec.Containers[i].Envs {
			env[e.Env] = e.Value
		}
		ci.Envs = env

		processImageVolumes(info, c.Id, p.spec, &p.spec.Containers[i])

		err = processInjectFiles(&p.spec.Containers[i], files, sd, c.Id, sd.RootPath(), sharedDir)
		if err != nil {
			return err
		}

		p.containers = append(p.containers, ci)
		glog.V(1).Infof("Container Info is \n%v", ci)
	}

	return nil
}

func getContinerInfo(dclient DockerInterface, container *hypervisor.Container) (info *dockertypes.ContainerJSON, err error) {
	info, err = dclient.GetContainerInfo(container.Id)
	if err != nil {
		glog.Error("got error when get container Info ", err.Error())
		return nil, err
	}
	if container.Name == "" {
		container.Name = info.Name
	}
	if container.Image == "" {
		container.Image = info.Config.Image
	}
	return
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

func processImageVolumes(config *dockertypes.ContainerJSON, id string, userPod *pod.UserPod, container *pod.UserContainer) {
	if config.Config.Volumes == nil {
		return
	}

	for tgt := range config.Config.Volumes {
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
		userPod.Volumes = append(userPod.Volumes, v)
		container.Volumes = append(container.Volumes, r)
	}
}

func (p *Pod) PrepareServices() error {
	err := servicediscovery.PrepareServices(p.spec, p.id)
	if err != nil {
		glog.Errorf("PrepareServices failed %s", err.Error())
	}
	return err
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
func (p *Pod) PrepareDNS() (err error) {
	err = nil
	var (
		resolvconf = "/etc/resolv.conf"
		fileId     = p.id + "-resolvconf"
	)

	if p.spec == nil {
		estr := "No Spec available for insert a DNS configuration"
		glog.V(1).Info(estr)
		err = fmt.Errorf(estr)
		return
	}

	if len(p.spec.Dns) > 0 {
		glog.V(1).Info("Already has DNS config, bypass DNS insert")
		return
	}

	if stat, e := os.Stat(resolvconf); e != nil || !stat.Mode().IsRegular() {
		glog.V(1).Info("Host resolv.conf is not exist or not a regular file, do not insert DNS conf")
		return
	}

	for _, src := range p.spec.Files {
		if src.Uri == "file:///etc/resolv.conf" {
			glog.V(1).Info("Already has resolv.conf configured, bypass DNS insert")
			return
		}
	}

	p.spec.Files = append(p.spec.Files, pod.UserFile{
		Name:     fileId,
		Encoding: "raw",
		Uri:      "file://" + resolvconf,
	})

	for idx, c := range p.spec.Containers {
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

		p.spec.Containers[idx].Files = append(c.Files, pod.UserFileReference{
			Path:     resolvconf,
			Filename: fileId,
			Perm:     "0644",
		})
	}

	return
}

func (p *Pod) PrepareVolume(daemon *Daemon, sd Storage) (err error) {
	err = nil
	p.volumes = []*hypervisor.VolumeInfo{}

	var (
		sharedDir = path.Join(hypervisor.BaseDir, p.vm.Id, hypervisor.ShareDirTag)
	)

	for _, v := range p.spec.Volumes {
		var vol *hypervisor.VolumeInfo
		if v.Source == "" {
			vol, err = sd.CreateVolume(daemon, p.id, v.Name)
			if err != nil {
				return
			}

			v.Source = vol.Filepath
			if sd.Type() != "devicemapper" {
				v.Driver = "vfs"

				vol.Filepath, err = storage.MountVFSVolume(v.Source, sharedDir)
				if err != nil {
					return
				}
				glog.V(1).Infof("dir %s is bound to %s", v.Source, vol.Filepath)

			} else { // type other than doesn't need to be mounted
				v.Driver = "raw"
			}
		} else {
			vol, err = ProbeExistingVolume(&v, sharedDir)
			if err != nil {
				return
			}
		}

		p.volumes = append(p.volumes, vol)
	}

	return nil
}

func (p *Pod) Prepare(daemon *Daemon) (err error) {
	if err = p.PrepareServices(); err != nil {
		return
	}

	if err = p.PrepareDNS(); err != nil {
		glog.Warning("Fail to prepare DNS for %s: %v", p.id, err)
		return
	}

	if err = p.PrepareContainers(daemon.Storage, daemon.DockerCli); err != nil {
		return
	}

	if err = p.PrepareVolume(daemon, daemon.Storage); err != nil {
		return
	}

	return nil
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
	if p.spec.LogConfig.Type == "" {
		p.spec.LogConfig.Type = daemon.DefaultLog.Type
		p.spec.LogConfig.Config = daemon.DefaultLog.Config
	}

	if p.spec.LogConfig.Type == "none" {
		return nil
	}

	var (
		needLogger []int = []int{}
		creator    logger.Creator
	)

	for i, c := range p.status.Containers {
		if c.Logs.Driver == nil {
			needLogger = append(needLogger, i)
		}
	}

	if len(needLogger) == 0 && p.status.Status == types.S_POD_RUNNING {
		return nil
	}

	if err = logger.ValidateLogOpts(p.spec.LogConfig.Type, p.spec.LogConfig.Config); err != nil {
		return
	}
	creator, err = logger.GetLogDriver(p.spec.LogConfig.Type)
	if err != nil {
		return
	}
	glog.V(1).Infof("configuring log driver [%s] for %s", p.spec.LogConfig.Type, p.id)

	for i, c := range p.status.Containers {
		ctx := logger.Context{
			Config:             p.spec.LogConfig.Config,
			ContainerID:        c.Id,
			ContainerName:      c.Name,
			ContainerImageName: p.spec.Containers[i].Image,
			ContainerCreated:   time.Now(), //FIXME: should record creation time in PodStatus
		}

		if p.containers != nil && len(p.containers) > i {
			ctx.ContainerEntrypoint = p.containers[i].Workdir
			ctx.ContainerArgs = p.containers[i].Cmd
			ctx.ContainerImageID = p.containers[i].Image
		}

		if p.spec.LogConfig.Type == jsonfilelog.Name {
			ctx.LogPath = filepath.Join(p.status.ResourcePath, fmt.Sprintf("%s-json.log", c.Id))
			glog.V(1).Info("configure container log to ", ctx.LogPath)
		}

		if c.Logs.Driver, err = creator(ctx); err != nil {
			return
		}
		glog.V(1).Infof("configured logger for %s/%s (%s)", p.id, c.Id, c.Name)
	}

	return nil
}

func (p *Pod) startLogging(daemon *Daemon) (err error) {
	err = nil

	if err = p.getLogger(daemon); err != nil {
		return
	}

	if p.spec.LogConfig.Type == "none" {
		return nil
	}

	for _, c := range p.status.Containers {
		var stdout, stderr io.Reader

		tag := "log-" + utils.RandStr(8, "alphanum")
		if stdout, stderr, err = p.vm.GetLogOutput(c.Id, tag, nil); err != nil {
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

func (p *Pod) AttachTtys(streams []*hypervisor.TtyIO) (err error) {

	ttyContainers := p.containers
	if p.spec.Type == "service-discovery" {
		ttyContainers = p.containers[1:]
	}

	for idx, str := range streams {
		if idx >= len(ttyContainers) {
			break
		}

		err = p.vm.Attach(str.Stdin, str.Stdout, str.ClientTag, ttyContainers[idx].Id, str.Callback, nil)
		if err != nil {
			glog.Errorf("Failed to attach client %s before start pod", str.ClientTag)
			return
		}
		glog.V(1).Infof("Attach client %s before start pod", str.ClientTag)
	}

	return nil
}

func (p *Pod) Start(daemon *Daemon, vmId string, lazy, autoremove bool, keep int, streams []*hypervisor.TtyIO) (*types.VmResponse, error) {

	var err error = nil

	if err = p.GetVM(daemon, vmId, lazy, keep); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil && vmId == "" {
			p.KillVM(daemon)
		}
	}()

	if err = p.Prepare(daemon); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			stopLogger(p.status)
		}
	}()

	if err = p.startLogging(daemon); err != nil {
		return nil, err
	}

	if err = p.AttachTtys(streams); err != nil {
		return nil, err
	}

	vmResponse := p.vm.StartPod(p.status, p.spec, p.containers, p.volumes)
	if vmResponse.Data == nil {
		err = fmt.Errorf("VM response data is nil")
		return vmResponse, err
	}

	err = daemon.UpdateVmData(p.vm.Id, vmResponse.Data.([]byte))
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}
	// add or update the Vm info for POD
	err = daemon.UpdateVmByPod(p.id, p.vm.Id)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	return vmResponse, nil
}

func (daemon *Daemon) GetExitCode(podId, tag string, callback chan *types.VmResponse) error {
	var (
		pod *Pod
		ok  bool
	)

	daemon.PodList.Lock()
	if pod, ok = daemon.PodList.Get(podId); !ok {
		daemon.PodList.Unlock()
		return fmt.Errorf("Can not find the POD instance of %s", podId)
	}
	if pod.vm == nil {
		daemon.PodList.Unlock()
		return fmt.Errorf("pod %s is already stopped", podId)
	}
	daemon.PodList.Unlock()
	return pod.vm.GetExitCode(tag, callback)
}

func (daemon *Daemon) StartPod(podId, podArgs, vmId string, config interface{}, lazy, autoremove bool, keep int, streams []*hypervisor.TtyIO) (int, string, error) {
	glog.V(1).Infof("podArgs: %s", podArgs)
	var (
		err error
		p   *Pod
	)

	p, err = daemon.GetPod(podId, podArgs, autoremove)
	if err != nil {
		return -1, "", err
	}

	if p.vm != nil {
		return -1, "", fmt.Errorf("pod %s is already running", podId)
	}

	vmResponse, err := p.Start(daemon, vmId, lazy, autoremove, keep, streams)
	if err != nil {
		return -1, "", err
	}

	return vmResponse.Code, vmResponse.Cause, nil
}

// The caller must make sure that the restart policy and the status is right to restart
func (daemon *Daemon) RestartPod(mypod *hypervisor.PodStatus) error {
	// Remove the pod
	// The pod is stopped, the vm is gone
	for _, c := range mypod.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
			glog.V(1).Infof("Error to rm container: %s", err.Error())
		}
	}
	daemon.RemovePod(mypod.Id)
	daemon.DeletePodContainerFromDB(mypod.Id)
	daemon.DeleteVolumeId(mypod.Id)

	podData, err := daemon.GetPodByName(mypod.Id)
	if err != nil {
		return err
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode()

	// Start the pod
	_, _, err = daemon.StartPod(mypod.Id, string(podData), "", nil, lazy, false, types.VM_KEEP_NONE, []*hypervisor.TtyIO{})
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	if err := daemon.WritePodAndContainers(mypod.Id); err != nil {
		glog.Error("Found an error while saving the Containers info")
		return err
	}

	return nil
}

func hyperHandlePodEvent(vmResponse *types.VmResponse, data interface{},
	mypod *hypervisor.PodStatus, vm *hypervisor.Vm) bool {
	daemon := data.(*Daemon)

	if vmResponse.Code == types.E_POD_FINISHED {
		if vm.Keep != types.VM_KEEP_NONE {
			vm.Status = types.S_VM_IDLE
			return false
		}
		stopLogger(mypod)
		mypod.SetPodContainerStatus(vmResponse.Data.([]uint32))
		vm.Status = types.S_VM_IDLE
		if mypod.Autoremove == true {
			daemon.CleanPod(mypod.Id)
			return false
		}
	} else if vmResponse.Code == types.E_VM_SHUTDOWN {
		if mypod.Status == types.S_POD_RUNNING {
			stopLogger(mypod)
			mypod.Status = types.S_POD_SUCCEEDED
			mypod.SetContainerStatus(types.S_POD_SUCCEEDED)
		}
		mypod.Vm = ""
		daemon.PodStopped(mypod.Id)
		if mypod.Type == "kubernetes" {
			switch mypod.Status {
			case types.S_POD_SUCCEEDED:
				if mypod.RestartPolicy == "always" {
					daemon.RestartPod(mypod)
					break
				}
				daemon.DeletePodFromDB(mypod.Id)
				for _, c := range mypod.Containers {
					glog.V(1).Infof("Ready to rm container: %s", c.Id)
					if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
						glog.V(1).Infof("Error to rm container: %s", err.Error())
					}
				}
				daemon.DeletePodContainerFromDB(mypod.Id)
				daemon.DeleteVolumeId(mypod.Id)
				break
			case types.S_POD_FAILED:
				if mypod.RestartPolicy != "never" {
					daemon.RestartPod(mypod)
					break
				}
				daemon.DeletePodFromDB(mypod.Id)
				for _, c := range mypod.Containers {
					glog.V(1).Infof("Ready to rm container: %s", c.Id)
					if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
						glog.V(1).Infof("Error to rm container: %s", err.Error())
					}
				}
				daemon.DeletePodContainerFromDB(mypod.Id)
				daemon.DeleteVolumeId(mypod.Id)
				break
			default:
				break
			}
		}
		return true
	}

	return false
}
