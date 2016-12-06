package pod

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/daemon/logger"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	runvtypes "github.com/hyperhq/runv/hypervisor/types"
)

const (
	//The Log level for Pods
	TRACE   = hlog.TRACE
	DEBUG   = hlog.DEBUG
	INFO    = hlog.INFO
	WARNING = hlog.WARNING
	ERROR   = hlog.ERROR
)

// PodState is the state of a Pod/Sandbox. in the current implementation, we assume the sandbox could be spawned fast
// enough and we don't need to pre-create a sandbox in the Pod view (or say, App view).
type PodState int32

const (
	S_POD_NONE     PodState = iota // DEFAULT
	S_POD_STARTING                 // vm context exist
	S_POD_RUNNING                  // sandbox inited,
	S_POD_PAUSED
	S_POD_STOPPED  // vm stopped, no vm associated
	S_POD_STOPPING // user initiates a stop/remove pod command
	S_POD_ERROR    // failed to stop/remove...
)

// XPod is the Pod keeper, or, the App View of a sandbox. All API for Pod operations or Container operations should be
// provided by this struct.
type XPod struct {
	// Name is the unique name of a pod provided by user
	name string

	// logPrefix is the prefix for log message
	logPrefix string

	// FIXME: should we get the pod id back, the problem of id is --- we have to maintain two unique index for pod,

	// globalSpec is the sandbox-wise spec, the stateful resources are not included in this field. see also:
	globalSpec *apitypes.UserPod

	// stateful resources:
	containers   map[string]*Container
	volumes      map[string]*Volume
	interfaces   map[string]*Interface
	services     []*apitypes.UserService
	portMappings []*apitypes.PortMapping
	labels       map[string]string
	resourceLock *sync.Mutex

	sandbox *hypervisor.Vm
	factory *PodFactory

	info       *apitypes.PodInfo
	status     PodState
	execs      map[string]*Exec
	statusLock *sync.RWMutex
	// stoppedChan: When the sandbox is down and the pod is stopped, a bool will be put into this channel,
	// if you want to do some op after the pod is clean stopped, just wait for this channel. And if an op
	// got a value from this chan, it should put an element to it again, in case other procedure may wait
	// on it too.
	stoppedChan chan bool
}

// The Log infrastructure, to add pod name as prefix of the log message.

// LogPrefix() belongs to the interface `github.com/hyperhq/hypercontainer-utils/hlog.LogOwner`, which helps `hlog.HLog` get
// proper prefix from the owner object.
func (p *XPod) LogPrefix() string {
	return p.logPrefix
}

// Log() employ `github.com/hyperhq/hypercontainer-utils/hlog.HLog` to add pod information to the log
func (p *XPod) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, p, 1, args...)
}

func (p *XPod) Id() string {
	return p.name
}

func (p *XPod) Name() string {
	return p.name
}

// SandboxName() returns the id of the sandbox, the detail of sandbox should be wrapped inside XPod, this method is
// used for list/display only.
func (p *XPod) SandboxName() string {
	var sbn = ""
	p.statusLock.RLock()
	if p.sandbox != nil {
		sbn = p.sandbox.Id
	}
	p.statusLock.RUnlock()
	return sbn
}

func (p *XPod) IsNone() bool {
	p.statusLock.Lock()
	isNone := p.status == S_POD_NONE
	p.statusLock.Unlock()

	return isNone
}

func (p *XPod) IsRunning() bool {
	p.statusLock.Lock()
	running := p.status == S_POD_RUNNING
	p.statusLock.Unlock()

	return running
}

func (p *XPod) IsAlive() bool {
	p.statusLock.Lock()
	alive := (p.status == S_POD_RUNNING) || (p.status == S_POD_STARTING)
	p.statusLock.Unlock()

	return alive
}

func (p *XPod) IsContainerRunning(cid string) bool {
	if !p.IsRunning() {
		return false
	}
	if c, ok := p.containers[cid]; ok {
		return c.IsRunning()
	}
	return false
}

func (p *XPod) IsContainerAlive(cid string) bool {
	if c, ok := p.containers[cid]; ok {
		return c.IsAlive()
	}
	return false
}

func (p *XPod) BriefStatus() (s *apitypes.PodListResult) {
	if p.info == nil {
		p.initPodInfo()
	}

	p.statusLock.RLock()
	s = &apitypes.PodListResult{
		PodID:     p.Id(),
		PodName:   p.Name(),
		VmID:      p.SandboxName(),
		CreatedAt: p.info.CreatedAt,
		Labels:    p.labels,
	}

	switch p.status {
	case S_POD_NONE:
		s.Status = ""
	case S_POD_STARTING:
		s.Status = "pending"
	case S_POD_RUNNING:
		s.Status = "running"
	case S_POD_STOPPED:
		s.Status = "failed"
	case S_POD_PAUSED:
		s.Status = "paused"
	case S_POD_STOPPING:
		s.Status = "stopping"
	case S_POD_ERROR:
		s.Status = "stopping"
	default:
	}
	p.statusLock.RUnlock()

	return s
}

func (p *XPod) StatusString() string {
	s := p.BriefStatus()
	return strings.Join([]string{s.PodID, s.PodName, s.VmID, s.Status}, ":")
}

func (p *XPod) SandboxBriefStatus() (s *apitypes.VMListResult) {
	p.statusLock.RLock()
	if p.sandbox != nil {
		s = &apitypes.VMListResult{
			VmID:  p.SandboxName(),
			PodID: p.Id(),
		}
		if p.status == S_POD_PAUSED {
			s.Status = "paused"
		} else {
			s.Status = "associated"
		}
	}
	p.statusLock.RUnlock()
	return s
}

func (p *XPod) SandboxStatusString() string {
	if s := p.SandboxBriefStatus(); s != nil {
		return strings.Join([]string{s.VmID, s.PodID, s.Status}, ":")
	}
	return ""
}

func (p *XPod) GetExitCode(cid, execId string) (uint8, error) {
	if execId != "" {
		return p.GetExecExitCode(cid, execId)
	}
	if c, ok := p.containers[cid]; ok {
		return c.GetExitCode()
	}
	err := fmt.Errorf("cannot find container %s", cid)
	p.Log(ERROR, "failed to get exit code: %v", err)
	return 255, err
}

func (p *XPod) ContainerBriefStatus(cid string) *apitypes.ContainerListResult {
	if c, ok := p.containers[cid]; ok {
		return c.BriefStatus()
	}
	return nil
}

func (p *XPod) ContainerStatusString(cid string) string {
	if c, ok := p.containers[cid]; ok {
		return c.StatusString()
	}
	return ""
}

func (p *XPod) ContainerHasTty(cid string) bool {
	if c, ok := p.containers[cid]; ok {
		return c.HasTty()
	}
	return false
}

func (p *XPod) Info() (*apitypes.PodInfo, error) {
	if p.status == S_POD_NONE {
		err := fmt.Errorf("pod not ready")
		p.Log(WARNING, err)
		return nil, err
	}

	if p.info == nil {
		p.initPodInfo()
	}

	p.updatePodInfo()

	return p.info, nil
}

func (p *XPod) ContainerInfo(cid string) (*apitypes.ContainerInfo, error) {
	if c, ok := p.containers[cid]; ok {
		ci := &apitypes.ContainerInfo{
			PodID:     p.Id(),
			Container: c.Info(),
			CreatedAt: c.CreatedAt().Unix(),
			Status:    c.InfoStatus(),
		}
		return ci, nil
	}
	err := fmt.Errorf("container %s does not existing", cid)
	p.Log(ERROR, err)
	return nil, err

}

func (p *XPod) Stats() *runvtypes.VmResponse {
	//use channel, don't block in statusLock
	ch := make(chan *runvtypes.VmResponse, 1)

	p.statusLock.Lock()
	if p.sandbox == nil {
		ch <- nil
	}
	go func(sb *hypervisor.Vm) {
		ch <- sb.Stats()
	}(p.sandbox)
	p.statusLock.Unlock()

	return <-ch
}

func (p *XPod) initPodInfo() {

	info := &apitypes.PodInfo{
		PodID:      p.Id(),
		PodName:    p.Name(),
		Kind:       "Pod",
		CreatedAt:  time.Now().UTC().Unix(),
		ApiVersion: utils.APIVERSION,
		Spec: &apitypes.PodSpec{
			Vcpu:   p.globalSpec.Resource.Vcpu,
			Memory: p.globalSpec.Resource.Memory,
			Labels: p.labels,
		},
		Status: &apitypes.PodStatus{
			HostIP: utils.GetHostIP(),
		},
	}
	if p.sandbox != nil {
		info.Vm = p.sandbox.Id
	}

	p.info = info
}

func (p *XPod) updatePodInfo() error {
	p.statusLock.Lock()
	defer p.statusLock.Unlock()

	var (
		containers      = make([]*apitypes.Container, 0, len(p.containers))
		volumes         = make([]*apitypes.PodVolume, 0, len(p.volumes))
		containerStatus = make([]*apitypes.ContainerStatus, 0, len(p.containers))
	)

	p.info.Spec.Labels = p.labels

	for _, v := range p.volumes {
		volumes = append(volumes, v.Info())
	}
	p.info.Spec.Volumes = volumes

	succeeeded := "Succeeded"
	for _, c := range p.containers {
		ci := c.Info()
		cs := c.InfoStatus()
		containers = append(containers, ci)
		containerStatus = append(containerStatus, cs)
		if cs.Phase == "failed" {
			succeeeded = "Failed"
		}
	}
	p.info.Spec.Containers = containers
	p.info.Status.ContainerStatus = containerStatus

	switch p.status {
	case S_POD_NONE:
		p.info.Status.Phase = "Pending"
	case S_POD_STARTING:
		p.info.Status.Phase = "Pending"
	case S_POD_RUNNING:
		p.info.Status.Phase = "Running"
	case S_POD_PAUSED:
		p.info.Status.Phase = "Paused"
	case S_POD_STOPPING:
		p.info.Status.Phase = "Running"
	case S_POD_STOPPED:
		p.info.Status.Phase = succeeeded
	case S_POD_ERROR:
		p.info.Status.Phase = "Failed"
	}
	if p.status == S_POD_RUNNING && p.sandbox != nil && len(p.info.Status.PodIP) == 0 {
		p.info.Status.PodIP = p.sandbox.GetIPAddrs()
	}

	return nil
}

func (p *XPod) HasServiceContainer() bool {
	return p.globalSpec.Type == "service-discovery" || len(p.services) > 0
}

func (p *XPod) ContainerLogger(id string) logger.Logger {
	c, ok := p.containers[id]
	if ok {
		return c.getLogger()
	}
	p.Log(WARNING, "connot get container %s for logger", id)
	return nil
}

func (p *XPod) SetLabel(labels map[string]string, update bool) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	p.Log(INFO, "update labels (ow: %v): %#v", update, labels)

	if p.labels == nil {
		p.labels = make(map[string]string)
	}

	if !update {
		for k := range labels {
			if _, ok := p.labels[k]; ok {
				return fmt.Errorf("Can't update label %s without override flag", k)
			}
		}
	}

	for k, v := range labels {
		p.labels[k] = v
	}

	return nil
}

func (p *XPod) ContainerIds() []string {
	result := make([]string, 0, len(p.containers))
	for cid := range p.containers {
		result = append(result, cid)
	}
	return result
}

func (p *XPod) ContainerNames() []string {
	result := make([]string, 0, len(p.containers))
	for _, c := range p.containers {
		result = append(result, c.SpecName())
	}
	return result
}

func (p *XPod) ContainerIdsOf(ctype apitypes.UserContainer_ContainerType) []string {
	result := make([]string, 0, len(p.containers))
	for cid, c := range p.containers {
		if c.spec.Type == ctype {
			result = append(result, cid)
		}
	}
	return result
}

func (p *XPod) ContainerName2Id(name string) (string, bool) {
	if name == "" {
		return "", false
	}

	if _, ok := p.containers[name]; ok {
		return name, true
	}

	for _, c := range p.containers {
		if name == c.SpecName() || strings.HasPrefix(c.Id(), name) {
			return c.Id(), true
		}
	}

	return "", false
}

func (p *XPod) ContainerId2Name(id string) string {
	if c, ok := p.containers[id]; ok {
		return c.SpecName()
	}
	return ""
}

func (p *XPod) Attach(cid string, stdin io.ReadCloser, stdout io.WriteCloser, rsp chan<- error) error {
	if !p.IsAlive() {
		err := fmt.Errorf("only alive container could be attached, current %v", p.status)
		p.Log(ERROR, err)
		return err
	}
	c, ok := p.containers[cid]
	if !ok {
		err := fmt.Errorf("container %s not exist", cid)
		p.Log(ERROR, err)
		return err
	}
	if !c.IsAlive() {
		err := fmt.Errorf("container is not available: %v", c.CurrentState())
		c.Log(ERROR, err)
		return err
	}

	return c.attach(stdin, stdout, nil, rsp)
}

func (p *XPod) TtyResize(cid, execId string, h, w int) error {
	if !p.IsAlive() || p.sandbox == nil {
		err := fmt.Errorf("only alive container could be attached, current %v", p.status)
		p.Log(ERROR, err)
		return err
	}
	_, ok := p.containers[cid]
	if !ok {
		err := fmt.Errorf("container %s not exist", cid)
		p.Log(ERROR, err)
		return err
	}
	return p.sandbox.Tty(cid, execId, h, w)
}

func (p *XPod) WaitContainer(cid string, second int) (int, error) {
	if !p.IsAlive() {
		err := fmt.Errorf("only alive container could be attached, current %v", p.status)
		p.Log(ERROR, err)
		return -1, err
	}
	c, ok := p.containers[cid]
	if !ok {
		err := fmt.Errorf("container %s not exist", cid)
		p.Log(ERROR, err)
		return -1, err
	}
	if c.IsStopped() || !c.IsRunning() {
		p.Log(DEBUG, "container is already stopped")
		return 0, nil
	}
	ch := p.sandbox.WaitProcess(true, []string{cid}, second)
	if ch == nil {
		c.Log(WARNING, "connot wait container, possiblely already down")
		return -1, nil
	}
	r, ok := <-ch
	if !ok {
		err := fmt.Errorf("break")
		c.Log(ERROR, "chan broken while waiting container")
		return -1, err
	}
	c.Log(INFO, "container stopped: %v", r.Code)
	return r.Code, nil
}

func (p *XPod) RenameContainer(cid, name string) error {
	var err error
	c, ok := p.containers[cid]
	if !ok {
		err = fmt.Errorf("container %s not found", cid)
		p.Log(ERROR, err)
		return err
	}
	old := c.SpecName()
	err = p.factory.registry.ReserveContainerName(name, p.Id())
	if err != nil {
		c.Log(ERROR, "failed to reserve new name during rename: %v", err)
		return err
	}
	defer func() {
		if err != nil {
			p.factory.registry.ReleaseContainerName(name)
		}
	}()

	err = c.rename(name)
	if err != nil {
		c.Log(ERROR, "failed to rename container: %v", err)
		return err
	}

	p.Log(INFO, "rename container from %s to %s", old, name)
	p.factory.registry.ReleaseContainerName(old)
	return nil
}
