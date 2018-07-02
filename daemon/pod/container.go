package pod

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/version"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/errors"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/term"
)

var epocZero = time.Time{}
var DetachKeys = "ctrl-p,ctrl-q"

type ContainerState int32

const (
	S_CONTAINER_NONE ContainerState = iota
	S_CONTAINER_CREATING
	S_CONTAINER_CREATED
	S_CONTAINER_RUNNING
	S_CONTAINER_STOPPING
)

type ContainerStatus struct {
	State      ContainerState
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Killed     bool

	sync.RWMutex
	stateChanged *sync.Cond
}

// A Container is run inside a Pod. It could be created as a member of a pod,
// and belongs to the pod until it is removed.
type Container struct {
	p *XPod

	spec     *apitypes.UserContainer
	descript *runv.ContainerDescription
	status   *ContainerStatus
	streams  *StreamConfig

	logger    LogStatus
	logPrefix string
}

func newContainerStatus() *ContainerStatus {
	cs := &ContainerStatus{
		State:      S_CONTAINER_NONE,
		CreatedAt:  epocZero,
		StartedAt:  epocZero,
		FinishedAt: epocZero,
	}
	cs.stateChanged = sync.NewCond(cs.RWMutex.RLocker())
	return cs
}

func newContainer(p *XPod, spec *apitypes.UserContainer, create bool) (*Container, error) {
	c := &Container{
		p:      p,
		spec:   spec,
		status: newContainerStatus(),
	}
	c.updateLogPrefix()
	if err := c.init(create); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Container) LogPrefix() string {
	return c.logPrefix
}

func (c *Container) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, c, 1, args...)
}

func (c *Container) updateLogPrefix() {
	if len(c.Id()) > 12 {
		id := c.Id()
		c.logPrefix = fmt.Sprintf("%sCon[%s(%s)] ", c.p.LogPrefix(), id[:12], c.SpecName())
	} else {
		c.logPrefix = fmt.Sprintf("%sCon[%s(%s)] ", c.p.LogPrefix(), c.Id(), c.SpecName())
	}
}

// Container Info:
func (c *Container) Id() string {
	return c.spec.Id
}

func (c *Container) SpecName() string {
	return c.spec.Name
}

func (c *Container) RuntimeName() string {
	if c.descript != nil {
		return c.descript.Name
	}
	return ""
}

func (c *Container) hasTty() bool {
	return c.spec.Tty
}

func (c *Container) CreatedAt() time.Time {
	c.status.RLock()
	ct := c.status.CreatedAt
	c.status.RUnlock()
	return ct
}

func (c *Container) Info() *apitypes.Container {
	c.status.RLock()
	defer c.status.RUnlock()
	cinfo := &apitypes.Container{
		Name:            c.RuntimeName(),
		ContainerID:     c.Id(),
		Image:           c.spec.Image,
		Commands:        c.spec.Command,
		WorkingDir:      c.spec.Workdir, //might be override by descript
		Labels:          c.spec.Labels,
		Ports:           make([]*apitypes.ContainerPort, 0, len(c.spec.Ports)),
		VolumeMounts:    make([]*apitypes.VolumeMount, 0, len(c.spec.Volumes)),
		Tty:             c.spec.Tty,
		ImagePullPolicy: "",
	}
	for _, port := range c.spec.Ports {
		cinfo.Ports = append(cinfo.Ports, &apitypes.ContainerPort{
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol,
		})
	}
	for _, vol := range c.spec.Volumes {
		cinfo.VolumeMounts = append(cinfo.VolumeMounts, &apitypes.VolumeMount{
			Name:      vol.Volume,
			MountPath: vol.Path,
			ReadOnly:  vol.ReadOnly,
		})
	}
	if c.descript != nil {
		cinfo.ImageID = c.descript.Image
		cinfo.Args = c.descript.Args
		cinfo.WorkingDir = c.descript.Workdir
		cinfo.Env = make([]*apitypes.EnvironmentVar, 0, len(c.descript.Envs))
		for e, v := range c.descript.Envs {
			cinfo.Env = append(cinfo.Env, &apitypes.EnvironmentVar{
				Env:   e,
				Value: v,
			})
		}
	} else {
		cinfo.Env = make([]*apitypes.EnvironmentVar, 0, len(c.spec.Envs))
		for _, env := range c.spec.Envs {
			cinfo.Env = append(cinfo.Env, &apitypes.EnvironmentVar{
				Env:   env.Env,
				Value: env.Value,
			})
		}

	}
	return cinfo
}

func (c *Container) InfoStatus() *apitypes.ContainerStatus {
	c.status.RLock()
	defer c.status.RUnlock()
	s := &apitypes.ContainerStatus{
		Name:        c.SpecName(),
		ContainerID: c.Id(),
		Waiting:     &apitypes.WaitingStatus{Reason: ""},
		Running:     &apitypes.RunningStatus{StartedAt: ""},
		Terminated:  &apitypes.TermStatus{},
	}
	switch c.status.State {
	case S_CONTAINER_NONE, S_CONTAINER_CREATING:
		s.Waiting.Reason = "Pending"
		s.Phase = "pending"
	case S_CONTAINER_CREATED:
		if c.status.FinishedAt != epocZero {
			s.Terminated.StartedAt = c.status.StartedAt.Format(time.RFC3339)
			s.Terminated.FinishedAt = c.status.FinishedAt.Format(time.RFC3339)
			s.Terminated.ExitCode = int32(c.status.ExitCode)
			if c.status.ExitCode != 0 || c.status.Killed {
				s.Terminated.Reason = "Failed"
				s.Phase = "failed"
			} else {
				s.Terminated.Reason = "Succeeded"
				s.Phase = "succeeded"
			}
		} else {
			s.Waiting.Reason = "Pending"
			s.Phase = "pending"
		}
	case S_CONTAINER_RUNNING, S_CONTAINER_STOPPING:
		s.Phase = "running"
		s.Running.StartedAt = c.status.StartedAt.Format(time.RFC3339)
	}
	c.Log(DEBUG, "retrive info %#v from status %#v", s, c.status)
	return s
}

// Container life cycle operations:
func (c *Container) Add() error {
	return nil
}

func (c *Container) start() error {
	if err := c.status.Start(); err != nil {
		if err == errors.ErrContainerAlreadyRunning {
			err = nil
			c.Log(INFO, "container in running state, do not need start")
			return nil
		}
		c.Log(ERROR, err)
		return err
	}

	c.startLogging()

	go c.waitFinish(-1)

	c.Log(INFO, "start container")
	if err := c.p.sandbox.StartContainer(c.Id()); err != nil {
		c.Log(ERROR, "failed to start container: %v", err)
		return err
	}
	c.Log(DEBUG, "container started")
	c.status.Running(time.Now())

	return nil
}

func (c *Container) setKill() {
	c.status.SetKilled()
	c.Log(DEBUG, "set container to be killed %#v", c.status)
}

func (c *Container) Remove() error {
	return nil
}

// Container operations:
func (c *Container) attach(stdin io.ReadCloser, stdout io.WriteCloser, rsp chan<- error) error {
	if c.p.sandbox == nil || c.descript == nil {
		err := fmt.Errorf("container not ready for attach")
		c.Log(ERROR, err)
		return err
	}

	var stderr io.Writer
	if stdout != nil {
		if !c.hasTty() {
			stderr = stdcopy.NewStdWriter(stdout, stdcopy.Stderr)
			stdout = &writeCloser{stdcopy.NewStdWriter(stdout, stdcopy.Stdout), stdout}
		}
	}
	detachKeys, _ := term.ToBytes(DetachKeys)

	go func() {
		rsp <- c.AttachStreams(true, true, c.hasTty(), stdin, stdout, stderr, detachKeys)
	}()
	return nil
}

// Container status
func (c *Container) CurrentState() ContainerState {
	c.status.RLock()
	current := c.status.State
	c.status.RUnlock()

	return current
}

func (c *Container) IsAlive() bool {
	c.status.RLock()
	alive := (c.status.State == S_CONTAINER_RUNNING || c.status.State == S_CONTAINER_CREATED || c.status.State == S_CONTAINER_CREATING)
	c.status.RUnlock()

	return alive
}

func (c *Container) IsRunning() bool {
	c.status.RLock()
	running := c.status.State == S_CONTAINER_RUNNING
	c.status.RUnlock()

	return running
}

func (c *Container) IsStopped() bool {
	c.status.RLock()
	stopped := c.status.State == S_CONTAINER_CREATED
	c.status.RUnlock()

	return stopped
}

func (c *Container) BriefStatus() (s *apitypes.ContainerListResult) {
	c.status.RLock()
	s = &apitypes.ContainerListResult{
		ContainerID:   c.Id(),
		ContainerName: c.SpecName(),
		PodID:         c.p.Id(),
	}
	switch c.status.State {
	case S_CONTAINER_NONE, S_CONTAINER_CREATING:
		s.Status = "pending"
	case S_CONTAINER_RUNNING, S_CONTAINER_STOPPING:
		s.Status = "running"
	case S_CONTAINER_CREATED:
		s.Status = "pending"
		if !c.status.FinishedAt.Equal(epocZero) {
			if c.status.ExitCode == 0 {
				s.Status = "succeeded"
			} else {
				s.Status = "failed"
			}
		}
	default:
	}
	c.status.RUnlock()
	return s
}

func (c *Container) StatusString() string {
	s := c.BriefStatus()
	return strings.Join([]string{s.ContainerID, s.ContainerName, s.PodID, s.Status}, ":")
}

func (c *Container) GetExitCode() (uint8, error) {
	c.status.RLock()
	for c.status.State != S_CONTAINER_CREATED {
		c.status.stateChanged.Wait()
	}
	code := uint8(c.status.ExitCode)
	if c.status.Killed {
		code = uint8(137)
	}
	c.status.RUnlock()
	return code, nil
}

func (c *Container) HasTty() bool {
	return c.spec.Tty
}

// Container resources
func (c *Container) init(allowCreate bool) error {
	var (
		cjson  *dockertypes.ContainerJSON
		err    error
		loaded bool
	)

	if c.spec.Name == "" {
		err = fmt.Errorf("no container name provided: %#v", c.spec)
		c.Log(ERROR, err)
		return err
	}

	if c.spec.Id != "" {
		cjson = c.loadJson()
		// if label tagged this is a new container, should set `loaded` false
		loaded = true
	}

	if cjson == nil {
		if !allowCreate {
			err = fmt.Errorf("could not load container")
			c.Log(ERROR, err)
			return err
		}

		cjson, err = c.createByEngine()
		if err != nil {
			c.Log(ERROR, err)
			return err
		}
	}

	c.status.CreatedAt, _ = time.Parse(time.RFC3339Nano, cjson.Created)

	desc, err := c.describeContainer(cjson)
	if err != nil {
		c.Log(ERROR, err)
		return err
	}

	c.mergeVolumes(cjson)

	if !loaded {
		if err = c.createVolumes(); err != nil {
			c.Log(ERROR, err)
			return err
		}

		// configEtcHosts should be called later than mergevolumes, guarantee the file is not described in container
		c.configEtcHosts()

		c.configDNS()
		c.injectFiles(desc.MountId)
	}

	desc.Volumes = c.parseVolumes()
	desc.Initialize = !loaded
	c.descript = desc

	return nil
}

func (c *Container) loadJson() *dockertypes.ContainerJSON {
	c.Log(TRACE, "Loading container")
	if r, err := c.p.factory.engine.ContainerInspect(c.spec.Id, false, version.Version("1.21")); err == nil {
		rsp, ok := r.(*dockertypes.ContainerJSON)
		if !ok {
			c.Log(ERROR, "fail to got loaded container info: %v", r)
			return nil
		}

		n := strings.TrimLeft(rsp.Name, "/")
		if c.spec.Name != n {
			c.Log(ERROR, "name mismatch of loaded container, loaded is %s", n)
			c.spec.Id = ""
			return nil
		}
		c.Log(DEBUG, "Found exist container")

		return rsp
	} else {
		c.Log(ERROR, "fail to load container: %v", err)
		return nil
	}
}

func (c *Container) createByEngine() (*dockertypes.ContainerJSON, error) {
	var (
		ok  bool
		err error
		ccs dockertypes.ContainerCreateResponse
		rsp *dockertypes.ContainerJSON
		r   interface{}
	)

	config := &container.Config{
		Image:           c.spec.Image,
		Cmd:             strslice.New(c.spec.Command...),
		NetworkDisabled: true,
	}

	if len(c.spec.Entrypoint) != 0 {
		config.Entrypoint = strslice.New(c.spec.Entrypoint...)
	}

	if c.spec.Workdir != "" {
		config.WorkingDir = c.spec.Workdir
	}

	if len(c.spec.Envs) != 0 {
		envs := []string{}
		for _, env := range c.spec.Envs {
			envs = append(envs, env.Env+"="+env.Value)
		}
		config.Env = envs
	}

	ccs, err = c.p.factory.engine.ContainerCreate(dockertypes.ContainerCreateConfig{
		Name:       c.spec.Name,
		Config:     config,
		HostConfig: &container.HostConfig{ReadonlyRootfs: c.spec.ReadOnly},
	})

	if err != nil {
		return nil, err
	}

	c.Log(INFO, "create container %s (w/: %s)", ccs.ID, ccs.Warnings)
	if r, err = c.p.factory.engine.ContainerInspect(ccs.ID, false, version.Version("1.21")); err != nil {
		return nil, err
	}

	if rsp, ok = r.(*dockertypes.ContainerJSON); !ok {
		err = fmt.Errorf("fail to unpack container json response for %s of %s", c.spec.Name, c.p.Id())
		return nil, err
	}

	c.spec.Id = ccs.ID
	c.updateLogPrefix()
	return rsp, nil
}

func (c *Container) describeContainer(cjson *dockertypes.ContainerJSON) (*runv.ContainerDescription, error) {

	c.Log(TRACE, "container info config %#v, Cmd %v, Args %v", cjson.Config, cjson.Config.Cmd.Slice(), cjson.Args)

	if c.spec.Image == "" {
		c.spec.Image = cjson.Config.Image
	}
	c.Log(INFO, "describe container")

	mountId, err := GetMountIdByContainer(c.p.factory.sd.Type(), c.spec.Id)
	if err != nil {
		err = fmt.Errorf("Cannot find mountID for container %s : %s", c.spec.Id, err)
		c.Log(ERROR, "Cannot find mountID for container %s", err)
		return nil, err
	}
	c.Log(DEBUG, "mount id: %s", mountId)

	cdesc := &runv.ContainerDescription{
		Id: c.spec.Id,

		Name:  cjson.Name, // will have a "/"
		Image: cjson.Image,

		Labels: c.spec.Labels,
		Tty:    c.spec.Tty,

		RootVolume: &runv.VolumeDescription{},
		MountId:    mountId,
		RootPath:   "rootfs",

		Envs:    make(map[string]string),
		Workdir: cjson.Config.WorkingDir,
		Path:    cjson.Path,
		Args:    cjson.Args,
		Rlimits: make([]*runv.Rlimit, 0, len(c.spec.Ulimits)),

		StopSignal: strings.ToUpper(cjson.Config.StopSignal),
	}

	//make sure workdir has an initial val
	if cdesc.Workdir == "" {
		cdesc.Workdir = "/"
	}

	for _, l := range c.spec.Ulimits {
		ltype := strings.ToLower(l.Name)
		cdesc.Rlimits = append(cdesc.Rlimits, &runv.Rlimit{
			Type: ltype,
			Hard: l.Hard,
			Soft: l.Soft,
		})
	}

	if c.spec.StopSignal != "" {
		cdesc.StopSignal = c.spec.StopSignal
	}
	if strings.HasPrefix(cdesc.StopSignal, "SIG") {
		cdesc.StopSignal = cdesc.StopSignal[len("SIG"):]
	}
	if cdesc.StopSignal == "" {
		cdesc.StopSignal = "TERM"
	}

	if c.spec.User != nil && c.spec.User.Name != "" {
		cdesc.UGI = &runv.UserGroupInfo{
			User:             c.spec.User.Name,
			Group:            c.spec.User.Group,
			AdditionalGroups: c.spec.User.AdditionalGroups,
		}
	} else if cjson.Config.User != "" {
		users := strings.Split(cjson.Config.User, ":")
		if len(users) > 2 {
			return nil, fmt.Errorf("container %s invalid user group config: %s", cjson.Name, cjson.Config.User)
		}
		if len(users) == 2 {
			cdesc.UGI = &runv.UserGroupInfo{
				User:  users[0],
				Group: users[1],
			}
		} else {
			cdesc.UGI = &runv.UserGroupInfo{
				User: cjson.Config.User,
			}
		}
	}

	for _, v := range cjson.Config.Env {
		pair := strings.SplitN(v, "=", 2)
		if len(pair) == 2 {
			cdesc.Envs[pair[0]] = pair[1]
		} else if len(pair) == 1 {
			cdesc.Envs[pair[0]] = ""
		}
	}

	c.Log(TRACE, "Container Info is \n%#v", cdesc)

	return cdesc, nil
}

func (c *Container) parseVolumes() map[string]*runv.VolumeReference {
	refs := make(map[string]*runv.VolumeReference)
	for _, vol := range c.spec.Volumes {
		if r, ok := refs[vol.Volume]; !ok {
			refs[vol.Volume] = &runv.VolumeReference{
				Name: vol.Volume,
				MountPoints: []*runv.VolumeMount{{
					Path:     vol.Path,
					ReadOnly: vol.ReadOnly,
				}},
			}
		} else {
			r.MountPoints = append(r.MountPoints, &runv.VolumeMount{
				Path:     vol.Path,
				ReadOnly: vol.ReadOnly,
			})
		}
	}
	return refs
}

func (c *Container) mergeVolumes(cjson *dockertypes.ContainerJSON) {

	if cjson == nil {
		return
	}

	existed := make(map[string]*apitypes.UserVolume)
	for _, vol := range c.spec.Volumes {
		existed[vol.Path] = vol.Detail
	}

	for tgt := range cjson.Config.Volumes {
		if _, ok := existed[tgt]; ok {
			continue
		}

		n := c.spec.Id + strings.Replace(tgt, "/", "_", -1)
		v := &apitypes.UserVolume{
			Name:   n,
			Source: "",
		}
		r := apitypes.UserVolumeReference{
			Volume:   n,
			Path:     tgt,
			ReadOnly: false,
			Detail:   v,
		}

		c.spec.Volumes = append(c.spec.Volumes, &r)
	}
}

func (c *Container) configEtcHosts() {
	var (
		hostsVolumeName = "etchosts-volume"
		hostsVolumePath = ""
		hostsPath       = "/etc/hosts"
	)

	for _, v := range c.spec.Volumes {
		if v.Path == hostsPath {
			return
		}
	}

	for _, f := range c.spec.Files {
		if f.Path == hostsPath {
			return
		}
	}

	_, hostsVolumePath = HostsPath(c.p.Id())

	vol := &apitypes.UserVolume{
		Name:   hostsVolumeName,
		Source: hostsVolumePath,
		Format: "vfs",
		Fstype: "dir",
	}

	ref := &apitypes.UserVolumeReference{
		Path:     hostsPath,
		Volume:   hostsVolumeName,
		ReadOnly: false,
		Detail:   vol,
	}

	c.spec.Volumes = append(c.spec.Volumes, ref)
}

func (c *Container) createVolumes() error {
	var (
		err     error
		created = []string{}
	)

	defer func() {
		if err != nil {
			for _, v := range created {
				c.p.factory.sd.RemoveVolume(c.p.Id(), []byte(v))
			}
		}
	}()

	for _, v := range c.spec.Volumes {
		if v.Detail == nil || v.Detail.Source != "" {
			continue
		}
		c.Log(INFO, "create volume %s", v.Volume)

		err = c.p.factory.sd.CreateVolume(c.p.Id(), v.Detail)
		if err != nil {
			c.Log(ERROR, "failed to create volume %s: %v", v.Volume, err)
			return err
		}
		created = append(created, v.Volume)
	}
	return nil
}

/***
  configDNS() Set the resolv.conf of host to each container, except the following cases:

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
func (c *Container) configDNS() {
	c.Log(DEBUG, "configure dns")
	var (
		resolvconf = "/etc/resolv.conf"
		fileId     = c.p.Id() + "-resolvconf"
	)

	if len(c.p.globalSpec.Dns) > 0 {
		c.Log(DEBUG, "Already has DNS config, bypass DNS insert")
		return
	}

	if stat, e := os.Stat(resolvconf); e != nil || !stat.Mode().IsRegular() {
		c.Log(DEBUG, "Host resolv.conf does not exist or not a regular file, do not insert DNS conf")
		return
	}

	for _, vol := range c.spec.Volumes {
		if vol.Path == resolvconf {
			c.Log(DEBUG, "Already has resolv.conf configured, bypass DNS insert")
			return
		}
	}

	for _, ref := range c.spec.Files {
		if ref.Path == resolvconf || (ref.Path+ref.Filename) == resolvconf ||
			(ref.Detail != nil && ref.Detail.Uri == "file:///etc/resolv.conf") {
			c.Log(DEBUG, "Already has resolv.conf configured, bypass DNS insert")
			return
		}
	}

	c.spec.Files = append(c.spec.Files, &apitypes.UserFileReference{
		Path:     resolvconf,
		Filename: fileId,
		Perm:     "0644",
		Detail: &apitypes.UserFile{
			Name:     fileId,
			Encoding: "raw",
			Uri:      "file://" + resolvconf,
		},
	})
}

func (c *Container) injectFiles(mountId string) error {
	if len(c.spec.Files) == 0 {
		return nil
	}

	var (
		sharedDir = filepath.Join(hypervisor.BaseDir, c.p.Id(), hypervisor.ShareDirTag)
	)

	for _, f := range c.spec.Files {
		targetPath := f.Path
		if strings.HasSuffix(targetPath, "/") {
			targetPath = targetPath + f.Filename
		}

		c.Log(DEBUG, "inject file %s", targetPath)
		if f.Detail == nil {
			c.Log(WARNING, "no file detail available, skip file %s injection", targetPath)
			continue
		}

		file := f.Detail
		var src io.Reader

		if file.Uri != "" {
			urisrc, err := utils.UriReader(file.Uri)
			if err != nil {
				return err
			}
			defer urisrc.Close()
			src = urisrc
		} else {
			src = strings.NewReader(file.Content)
		}

		switch file.Encoding {
		case "base64":
			src = base64.NewDecoder(base64.StdEncoding, src)
		default:
		}

		err := c.p.factory.sd.InjectFile(src, mountId, targetPath, sharedDir,
			utils.PermInt(f.Perm), utils.UidInt(f.User), utils.UidInt(f.Group))
		if err != nil {
			c.Log(ERROR, "got error when inject files: %v", err)
			return err
		}
	}

	return nil
}

func (c *Container) volumes() []*apitypes.UserVolume {
	var (
		result  = []*apitypes.UserVolume{}
		existed = make(map[string]bool)
	)

	for _, v := range c.spec.Volumes {
		if existed[v.Volume] || v.Detail == nil {
			continue
		}
		result = append(result, v.Detail)
		existed[v.Volume] = true
	}

	return result
}

func (c *Container) addToSandbox() error {
	var (
		volmap = make(map[string]bool)
		wg     = &utils.WaitGroupWithFail{}
	)
	c.Log(DEBUG, "begin add to sandbox")
	c.status.Create()
	for _, v := range c.spec.Volumes {
		if volmap[v.Volume] {
			continue
		}
		if vol, ok := c.p.volumes[v.Volume]; ok {
			volmap[v.Volume] = true
			if err := vol.subscribeInsert(wg); err != nil {
				c.Log(ERROR, "container depends on an impossible volume: %v", err)
				return err
			}
		}
	}

	root, err := c.p.factory.sd.PrepareContainer(c.descript.MountId, c.p.sandboxShareDir(), c.spec.ReadOnly)
	if err != nil {
		c.Log(ERROR, "failed to prepare rootfs: %v", err)
		return err
	}
	c.descript.RootVolume = root

	c.Log(TRACE, "finished container prepare, wait for volumes")
	if len(volmap) > 0 {
		err := wg.Wait()
		if err != nil {
			c.Log(ERROR, "failed to add volume: %v", err)
			return err
		}
	}

	c.Log(DEBUG, "resources ready, insert container to sandbox")
	r := c.p.sandbox.AddContainer(c.descript)
	if !r.IsSuccess() {
		err := fmt.Errorf("failed to add container to sandbox: %s", r.Message())
		c.Log(ERROR, err)
		c.status.UnexpectedStopped()
		return err
	}

	c.status.Created(time.Now())
	return nil
}

func (c *Container) associateToSandbox() error {
	c.Log(DEBUG, "try to associate container %s to sandbox", c.Id())
	alive, err := c.p.sandbox.AssociateContainer(c.Id())
	if err != nil {
		return err
	}
	// FIXME missing container status history here.
	if alive {
		c.status.State = S_CONTAINER_RUNNING
		c.status.StartedAt = time.Now()
	} else {
		c.status.State = S_CONTAINER_CREATED
		c.status.CreatedAt = time.Now()
	}

	go c.waitFinish(-1)

	c.startLogging()

	return nil
}

// This method should be called when initialzing container or put into resource lock.
func (c *Container) initStreams() error {
	if c.streams != nil {
		return nil
	}
	if !c.p.IsAlive() {
		c.Log(ERROR, "can not init stream to a non-existing sandbox")
		return errors.ErrPodNotAlive.WithArgs(c.p.Id())
	}
	c.Log(TRACE, "init io streams")
	c.streams = NewStreamConfig()
	c.streams.NewInputPipes()
	tty := &hypervisor.TtyIO{
		Stdin:  c.streams.Stdin(),
		Stdout: c.streams.Stdout(),
		Stderr: c.streams.Stderr(),
	}
	if err := c.p.sandbox.Attach(tty, c.Id()); err != nil {
		c.Log(ERROR, err)
		return err
	}
	return nil
}

func (c *Container) initLogger() {
	if c.logger.Driver != nil {
		return
	}

	if c.p.factory.logCreator == nil {
		return
	}

	ctx := logger.Context{
		Config:              c.p.factory.logCfg.Config,
		ContainerID:         c.Id(),
		ContainerName:       c.RuntimeName(),
		ContainerImageName:  c.descript.Image,
		ContainerCreated:    c.status.CreatedAt,
		ContainerEntrypoint: c.descript.Path,
		ContainerArgs:       c.descript.Args,
		ContainerImageID:    c.descript.Image,
	}

	if c.p.factory.logCfg.Type == jsonfilelog.Name {
		var logdir string

		if c.spec.LogPath != "" {
			ctx.LogPath = c.spec.LogPath
			logdir = filepath.Dir(ctx.LogPath)
		} else {
			logdir = c.p.factory.logCfg.PathPrefix
			if c.p.factory.logCfg.PodIdInPath {
				logdir = filepath.Join(logdir, c.p.Id())
			}
			ctx.LogPath = filepath.Join(logdir, fmt.Sprintf("%s-json.log", c.Id()))
		}

		if err := os.MkdirAll(logdir, os.FileMode(0755)); err != nil {
			c.Log(ERROR, "cannot create container log dir %s: %v", logdir, err)
			return
		}

		c.Log(DEBUG, "configure container log to %s", ctx.LogPath)
	}

	driver, err := c.p.factory.logCreator(ctx)
	if err != nil {
		return
	}
	c.logger.Driver = driver
	c.Log(DEBUG, "container logger configured")

	return
}

func (c *Container) startLogging() {
	c.initLogger()

	if c.logger.Driver == nil {
		return
	}

	var (
		stdout, stderr         io.Reader
		stdoutStub, stderrStub io.WriteCloser
		sources                = map[string]io.Reader{}
	)

	stdout, stdoutStub = io.Pipe()
	sources["stdout"] = stdout

	if !c.hasTty() {
		stderr, stderrStub = io.Pipe()
		sources["stderr"] = stderr
	}

	go c.AttachStreams(false, false, c.hasTty(), nil, stdoutStub, stderrStub, nil)
	c.logger.Copier = logger.NewCopier(c.Id(), sources, c.logger.Driver)
	c.logger.Copier.Run()

	if jl, ok := c.logger.Driver.(*jsonfilelog.JSONFileLogger); ok {
		c.logger.LogPath = jl.LogPath()
	}

	return
}

func (c *Container) getLogger() logger.Logger {
	if c.logger.Driver == nil && c.p.factory.logCreator != nil {
		c.initLogger()
	}
	return c.logger.Driver
}

func (c *Container) waitFinish(timeout int) {
	var firstStop bool

	result := c.p.sandbox.WaitProcess(true, []string{c.Id()}, timeout)
	if result == nil {
		c.Log(INFO, "wait container failed")
		firstStop = c.status.UnexpectedStopped()
	} else {
		r, ok := <-result
		if !ok {
			if timeout < 0 {
				c.Log(INFO, "container unexpected failed, chan broken")
				firstStop = c.status.UnexpectedStopped()
			}
		} else {
			c.Log(INFO, "container exited with code %v (at %v)", r.Code, r.FinishedAt)
			firstStop = c.status.Stopped(r.FinishedAt, r.Code)
		}
	}

	if firstStop {
		c.Log(INFO, "clean up container")

		//reset streams and loggers, in case restart may use them.
		oldStreams := c.streams
		oldLogger := c.logger.Driver
		c.streams = nil
		c.logger.Driver = nil

		//the streams should have been closed as the process terminated and hup the streams,
		//however, we reclaim them in case they are leaved alone by some reason
		time.AfterFunc(3*time.Second, func() {
			if err := oldStreams.CloseStreams(); err != nil {
				c.Log(WARNING, "failed to clean stopped streams of container: %v", err)
				err = nil
			}
			if oldLogger != nil {
				oldLogger.Close()
			}
		})
	}
}

func (c *Container) terminate(force bool) (err error) {
	if c.descript == nil {
		return
	}

	defer func() {
		if pe := recover(); pe != nil {
			err = fmt.Errorf("panic during killing container: %v", pe)
			c.Log(ERROR, err)
		}
	}()

	sig := syscall.SIGKILL
	if !force {
		sig = utils.StringToSignal(c.descript.StopSignal)
	}
	c.setKill()
	c.Log(DEBUG, "stopping: killing container with %d", sig)
	err = c.p.sandbox.KillContainer(c.Id(), sig)
	if err != nil {
		c.Log(ERROR, "failed to kill container: %v", err)
	}

	return err
}

func (c *Container) removeFromSandbox() error {
	r := c.p.sandbox.RemoveContainer(c.Id())
	if !r.IsSuccess() {
		err := fmt.Errorf("failed to remove container: %s", r.Message())
		c.Log(ERROR, err)
		return err
	}
	c.Log(DEBUG, "removed container from sandbox")
	return nil
}

func (c *Container) umountRootVol() error {
	if c.descript == nil || c.descript.MountId == "" {
		c.Log(DEBUG, "no root volume to umount")
		return nil
	}
	err := c.p.factory.sd.CleanupContainer(c.descript.MountId, c.p.sandboxShareDir())
	if err != nil {
		c.Log(ERROR, "failed to umount root volume: %v", err)
		return err
	}
	c.Log(DEBUG, "umounted root volume")
	return nil
}

func (c *Container) rename(name string) error {
	var err error
	old := c.SpecName()
	if name[0] == '/' {
		name = name[1:]
	}
	if !utils.DockerRestrictedNamePattern.MatchString(name) {
		err = fmt.Errorf("Invalid container name (%s), only %s are allowed", name, utils.DockerRestrictedNameChars)
		c.Log(ERROR, err)
		return err
	}
	if err := c.p.factory.registry.ReserveContainerName(name, c.p.Id()); err != nil {
		c.Log(ERROR, "failed to reserve new container name %s: %v", name, err)
		return err
	}
	defer func() {
		if err != nil {
			c.p.factory.registry.ReleaseContainerName(name)
		}
	}()
	if c.Id() != "" || c.descript != nil {
		err = c.p.factory.engine.ContainerRename(old, name)
		if err != nil {
			return err
		}
	}
	c.p.factory.registry.ReleaseContainerName(old)
	c.spec.Name = name
	if c.descript != nil {
		c.descript.Name = "/" + name
	}
	return err
}

func (c *Container) removeFromEngine() error {
	return c.p.factory.engine.ContainerRm(c.Id(), &dockertypes.ContainerRmConfig{})
}

// container status transition
func (cs *ContainerStatus) Create() error {
	cs.Lock()
	defer cs.Unlock()

	if cs.State != S_CONTAINER_NONE {
		err := fmt.Errorf("only NONE container could be create, current: %d", cs.State)
		return err
	}

	cs.State = S_CONTAINER_CREATING
	cs.stateChanged.Broadcast()

	return nil
}

func (cs *ContainerStatus) Created(t time.Time) error {
	cs.Lock()
	defer cs.Unlock()
	if cs.State != S_CONTAINER_CREATING {
		return fmt.Errorf("only CREATING container could be set to creatd, current: %d", cs.State)
	}

	cs.State = S_CONTAINER_CREATED
	cs.CreatedAt = t
	cs.stateChanged.Broadcast()

	return nil
}

func (cs *ContainerStatus) Start() error {
	cs.Lock()
	defer cs.Unlock()

	if cs.State == S_CONTAINER_RUNNING {
		return errors.ErrContainerAlreadyRunning
	} else if cs.State != S_CONTAINER_CREATED {
		return fmt.Errorf("only CREATED container could be set to running, current: %d", cs.State)
	}

	cs.Killed = false
	cs.State = S_CONTAINER_RUNNING
	cs.stateChanged.Broadcast()

	return nil
}

func (cs *ContainerStatus) SetKilled() {
	cs.Lock()
	cs.Killed = true
	cs.Unlock()
}

func (cs *ContainerStatus) Running(t time.Time) error {
	cs.Lock()
	defer cs.Unlock()

	if cs.State != S_CONTAINER_RUNNING {
		return fmt.Errorf("only RUNNING container could set started time, current: %d", cs.State)
	}
	cs.StartedAt = t
	return nil
}

func (cs *ContainerStatus) Stop() error {
	cs.Lock()
	defer cs.Unlock()

	if cs.State != S_CONTAINER_RUNNING {
		return fmt.Errorf("only RUNNING container could be stopped, current: %d", cs.State)
	}
	cs.State = S_CONTAINER_STOPPING
	cs.stateChanged.Broadcast()
	return nil
}

func (cs *ContainerStatus) Stopped(t time.Time, exitCode int) bool {
	var result bool
	cs.Lock()
	if cs.State == S_CONTAINER_RUNNING || cs.State == S_CONTAINER_STOPPING {
		cs.FinishedAt = t
		cs.ExitCode = exitCode
		result = true
	}
	cs.State = S_CONTAINER_CREATED
	cs.stateChanged.Broadcast()
	cs.Unlock()
	return result
}

func (cs *ContainerStatus) UnexpectedStopped() bool {
	return cs.Stopped(time.Now(), 255)
}

func (cs *ContainerStatus) IsRunning() bool {
	cs.RLock()
	defer cs.RUnlock()

	return cs.State == S_CONTAINER_RUNNING
}

func (cs *ContainerStatus) IsStopped() bool {
	cs.RLock()
	defer cs.RUnlock()

	return cs.State == S_CONTAINER_CREATED
}

// AttachStreams connects streams to a TTY.
// Used by exec too. Should this move somewhere else?
func (c *Container) AttachStreams(openStdin, stdinOnce, tty bool, stdin io.ReadCloser, stdout io.WriteCloser, stderr io.Writer, keys []byte) error {
	var (
		cStdout, cStderr io.ReadCloser
		cStdin           io.WriteCloser
		wg               sync.WaitGroup
		errors           = make(chan error, 3)
	)

	c.status.Lock()
	err := c.initStreams()
	streamConfig := c.streams
	c.status.Unlock()
	if err != nil {
		c.Log(ERROR, "failed to init streams during attach: %v", err)
		return err
	}

	if stdin != nil && openStdin {
		cStdin = streamConfig.StdinPipe()
		wg.Add(1)
	}

	if stdout != nil {
		cStdout = streamConfig.StdoutPipe()
		wg.Add(1)
	}

	if stderr != nil {
		cStderr = streamConfig.StderrPipe()
		wg.Add(1)
	}

	// Connect stdin of container to the http conn.
	go func() {
		if stdin == nil || !openStdin {
			return
		}
		c.Log(DEBUG, "attach: stdin: begin")

		var err error
		if tty {
			_, err = copyEscapable(cStdin, stdin, keys)
		} else {
			_, err = io.Copy(cStdin, stdin)
		}
		if err == io.ErrClosedPipe {
			err = nil
		}
		if err != nil {
			c.Log(ERROR, "attach: stdin: %s", err)
			errors <- err
		}
		if stdinOnce && !tty {
			cStdin.Close()
		}
		c.Log(DEBUG, "attach: stdin: end")
		wg.Done()
	}()

	attachStream := func(name string, stream io.Writer, streamPipe io.ReadCloser) {
		if stream == nil {
			return
		}

		c.Log(DEBUG, "attach: %s: begin", name)
		_, err := io.Copy(stream, streamPipe)
		if err == io.ErrClosedPipe {
			err = nil
		}
		if err != nil {
			c.Log(ERROR, "attach: %s: %v", name, err)
			errors <- err
		}
		// Make sure stdin gets closed
		if stdin != nil {
			stdin.Close()
		}
		streamPipe.Close()
		if closer, ok := stream.(io.Closer); ok {
			closer.Close()
		}
		c.Log(DEBUG, "attach: %s: end", name)
		wg.Done()
	}

	go attachStream("stdout", stdout, cStdout)
	go attachStream("stderr", stderr, cStderr)

	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

var DetachError error = fmt.Errorf("DetachError")

// Code c/c from io.Copy() modified to handle escape sequence
func copyEscapable(dst io.Writer, src io.ReadCloser, keys []byte) (written int64, err error) {
	if len(keys) == 0 {
		// Default keys : ctrl-p ctrl-q
		keys = []byte{16, 17}
	}
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			// ---- Docker addition
			preservBuf := []byte{}
			for i, key := range keys {
				preservBuf = append(preservBuf, buf[0:nr]...)
				if nr != 1 || buf[0] != key {
					break
				}
				if i == len(keys)-1 {
					src.Close()
					return 0, DetachError
				}
				nr, er = src.Read(buf)
			}
			var nw int
			var ew error
			if len(preservBuf) > 0 {
				nw, ew = dst.Write(preservBuf)
				nr = len(preservBuf)
			} else {
				// ---- End of docker
				nw, ew = dst.Write(buf[0:nr])
			}
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}
