package pod

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/errors"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor"
)

var (
	ProvisionTimeout = 5 * time.Minute
)

func CreateXPod(factory *PodFactory, spec *apitypes.UserPod) (*XPod, error) {

	p, err := newXPod(factory, spec)
	if err != nil {
		return nil, err
	}
	err = p.reserveNames(spec.Containers)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			p.releaseNames(spec.Containers)
		}
	}()
	err = p.createSandbox(spec) //TODO: add defer for rollback
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil && p.sandbox != nil {
			p.sandbox.Kill()
		}
	}()

	err = p.initResources(spec, true)
	if err != nil {
		return nil, err
	}

	err = p.prepareResources()
	if err != nil {
		return nil, err
	}

	err = p.addResourcesToSandbox()
	if err != nil {
		return nil, err
	}

	p.initPodInfo()

	// reserve again in case container is created
	err = p.reserveNames(spec.Containers)
	if err != nil {
		return nil, err
	}

	if err = p.savePod(); err != nil {
		return nil, err
	}

	return p, nil
}

func newXPod(factory *PodFactory, spec *apitypes.UserPod) (*XPod, error) {
	if err := spec.MergePortmappings(); err != nil {
		hlog.Log(ERROR, "fail to merge the portmappings: %v", err)
		return nil, err
	}
	if err := spec.ReorganizeContainers(true); err != nil {
		hlog.Log(ERROR, err)
		return nil, err
	}
	factory.hosts = HostsCreator(spec.Id)
	factory.logCreator = initLogCreator(factory, spec)
	p := &XPod{
		name:          spec.Id,
		logPrefix:     fmt.Sprintf("Pod[%s] ", spec.Id),
		globalSpec:    spec.CloneGlobalPart(),
		containers:    make(map[string]*Container),
		volumes:       make(map[string]*Volume),
		interfaces:    make(map[string]*Interface),
		portMappings:  spec.Portmappings,
		labels:        spec.Labels,
		prestartExecs: [][]string{},
		execs:         make(map[string]*Exec),
		resourceLock:  &sync.Mutex{},
		statusLock:    &sync.RWMutex{},
		stoppedChan:   make(chan bool, 1),
		factory:       factory,
	}
	p.initCond = sync.NewCond(p.statusLock.RLocker())
	return p, nil
}

func (p *XPod) ContainerCreate(c *apitypes.UserContainer) (string, error) {
	if !p.IsAlive() {
		err := fmt.Errorf("pod is not running")
		p.Log(ERROR, err)
		return "", err
	}

	if c.Name == "" {
		_, img, _ := utils.ParseImageRepoTag(c.Image)
		if !utils.IsDNSLabel(img) {
			img = ""
		}

		c.Name = fmt.Sprintf("%s-%s-%s", p.Name(), img, utils.RandStr(10, "alpha"))
	}

	if err := p.factory.registry.ReserveContainerName(c.Name, p.Id()); err != nil {
		p.Log(ERROR, "could not reserve name %s: %v", c.Name, err)
		return "", nil
	}

	p.resourceLock.Lock()
	id, err := p.doContainerCreate(c)
	p.factory.registry.ReserveContainerID(c.Id, p.Id())
	p.resourceLock.Unlock()

	return id, err
}

func (p *XPod) doContainerCreate(c *apitypes.UserContainer) (string, error) {
	pc, err := newContainer(p, c, true)
	if err != nil {
		p.Log(ERROR, "failed to create container %s: %v", c.Name, err)
		return "", err
	}

	p.containers[pc.Id()] = pc

	vols := pc.volumes()
	nvs := make([]string, 0, len(vols))
	for _, vol := range vols {
		if _, ok := p.volumes[vol.Name]; ok {
			pc.Log(TRACE, "volume %s has already been included, don't need to be inserted again", vol.Name)
			continue
		}
		p.volumes[vol.Name] = newVolume(p, vol)
		nvs = append(nvs, vol.Name)
	}
	pc.Log(TRACE, "volumes to be added: %v", nvs)

	future := utils.NewFutureSet()
	for _, vol := range nvs {
		future.Add(vol, p.volumes[vol].add)
	}
	future.Add(pc.Id(), pc.addToSandbox)
	if err := future.Wait(ProvisionTimeout); err != nil {
		p.Log(ERROR, "error during add container resources to sandbox: %v", err)
		return "", err
	}

	// serialize all changes to daemonDB
	for _, vn := range nvs {
		if err = p.volumes[vn].saveVolume(); err != nil {
			return "", err
		}
	}
	if err = pc.saveContainer(); err != nil {
		return "", err
	}
	if err = p.saveLayout(); err != nil {
		return "", err
	}
	if err = p.saveSandbox(); err != nil {
		p.Log(ERROR, "error during save sandbox: %v", err)
		return "", err
	}
	return pc.Id(), nil
}

func (p *XPod) ContainerStart(cid string) error {
	var err error
	c, ok := p.containers[cid]
	if !ok {
		err = fmt.Errorf("container %s not found", cid)
		p.Log(ERROR, err)
		return err
	}

	if c.IsRunning() {
		c.Log(INFO, "starting a running container")
		return nil
	}

	if !p.IsAlive() || !c.IsStopped() {
		err = fmt.Errorf("not ready for start p: %v, c: %v", p.status, c.CurrentState())
		c.Log(ERROR, err)
		return err
	}

	if err = c.start(); err != nil {
		return err
	}

	// save Sandbox state for attachID changed.
	return p.saveSandbox()
}

// Start() means start a STOPPED pod.
func (p *XPod) Start() error {

	if p.IsStopped() {
		if err := p.createSandbox(p.globalSpec); err != nil {
			p.Log(ERROR, "failed to create sandbox for the stopped pod: %v", err)
			return err
		}

		if err := p.prepareResources(); err != nil {
			return err
		}

		if err := p.addResourcesToSandbox(); err != nil {
			return err
		}
	}

	err := p.waitPodRun("start pod")
	if err != nil {
		p.Log(ERROR, "wait running failed, cannot start pod")
		return err
	}
	if err := p.startAll(); err != nil {
		return err
	}

	return p.saveSandbox()
}

func (p *XPod) createSandbox(spec *apitypes.UserPod) error {
	//in the future, here
	sandbox, err := startSandbox(p.factory.vmFactory, int(spec.Resource.Vcpu), int(spec.Resource.Memory), "", "")
	if err != nil {
		p.Log(ERROR, err)
		return err
	}
	if sandbox == nil {
		p.Log(ERROR, "startSandbox returns no sandbox and no error")
		return errors.ErrSandboxNotExist
	}

	config := &runv.SandboxConfig{
		Hostname:   spec.Hostname,
		Dns:        spec.Dns,
		DnsOptions: spec.DnsOptions,
		DnsSearch:  spec.DnsSearch,
		Neighbors: &runv.NeighborNetworks{
			InternalNetworks: spec.PortmappingWhiteLists.InternalNetworks,
			ExternalNetworks: spec.PortmappingWhiteLists.ExternalNetworks,
		},
	}

	p.sandbox = sandbox
	p.status = S_POD_STARTING

	go p.waitVMStop()
	err = sandbox.InitSandbox(config)
	if err != nil {
		go sandbox.Shutdown()
	}
	p.Log(INFO, "sandbox init result: %#v", err)
	p.setPodInitStatus(err == nil)
	return err
}

func (p *XPod) reconnectSandbox(sandboxId string, pinfo []byte) error {
	var (
		sandbox *hypervisor.Vm
		err     error
	)

	if sandboxId != "" {
		sandbox, err = hypervisor.AssociateVm(sandboxId, pinfo)
		if err != nil {
			p.Log(ERROR, err)
			sandbox = nil
		}
	}

	if sandbox == nil {
		p.status = S_POD_STOPPED
		return err
	}

	p.status = S_POD_RUNNING
	p.sandbox = sandbox
	go p.waitVMStop()
	return nil
}

func (p *XPod) setPodInitStatus(initSuccess bool) {
	if initSuccess && p.status == S_POD_RUNNING {
		return
	}
	p.statusLock.Lock()
	if initSuccess {
		if p.status == S_POD_STARTING {
			p.status = S_POD_RUNNING
		}
	} else {
		p.status = S_POD_STOPPING
	}
	p.initCond.Broadcast()
	p.statusLock.Unlock()
}

func (p *XPod) reserveNames(containers []*apitypes.UserContainer) error {
	var (
		err  error
		done = make([]*apitypes.UserContainer, 0, len(containers))
	)
	if err = p.factory.registry.ReservePod(p); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			p.releaseNames(done)
		}
	}()
	for _, c := range containers {
		if err = p.factory.registry.ReserveContainer(c.Id, c.Name, p.Id()); err != nil {
			p.Log(ERROR, err)
			return err
		}
		done = append(done, c)
	}
	return nil
}

func (p *XPod) releaseNames(containers []*apitypes.UserContainer) {
	for _, c := range containers {
		p.factory.registry.ReleaseContainer(c.Id, c.Name)
	}
	p.factory.registry.Release(p.Id())
}

// initResources() will create volumes, insert files etc. if needed.
// we can treat this function as an pre-processor of the spec
//
// If specify `allowCreate=true`, i.e. create rather than load, it will fill
// all the required fields, such as if an volume source is empty, this
// function will create the volume and fill the related fields.
//
// This function will do resource op and update the spec. and won't
// access sandbox.
func (p *XPod) initResources(spec *apitypes.UserPod, allowCreate bool) error {
	for _, cspec := range spec.Containers {
		c, err := newContainer(p, cspec, allowCreate)
		if err != nil {
			return err
		}
		p.containers[c.Id()] = c

		vols := c.volumes()
		for _, vol := range vols {
			if _, ok := p.volumes[vol.Name]; ok {
				continue
			}
			p.volumes[vol.Name] = newVolume(p, vol)
		}
	}

	if len(spec.Interfaces) == 0 {
		spec.Interfaces = append(spec.Interfaces, &apitypes.UserInterface{})
	}
	for _, nspec := range spec.Interfaces {
		inf := newInterface(p, nspec)
		p.interfaces[nspec.Ifname] = inf
	}

	p.services = newServices(p, spec.Services)

	return nil
}

// prepareResources() will allocate IP.
// This apply for creating and restart a stopped pod.
func (p *XPod) prepareResources() error {
	var (
		err error
	)

	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if !p.IsAlive() {
		p.Log(ERROR, "pod is not alive, can not prepare resources")
		return errors.ErrPodNotAlive.WithArgs(p.Id())
	}

	//generate /etc/hosts
	p.factory.hosts.Do()

	defer func() {
		if err != nil {
			for _, inf := range p.interfaces {
				inf.cleanup()
			}
		}
	}()

	for _, inf := range p.interfaces {
		if err = inf.prepare(); err != nil {
			return err
		}
		if p.containerIP == "" {
			p.containerIP = inf.descript.Ip
		}
	}

	err = p.initPortMapping()
	if err != nil {
		p.Log(ERROR, "failed to initial setup port mappings: %v", err)
		return err
	}
	// if insert any other operations here, add rollback code for the
	// port mapping operation

	return nil
}

// addResourcesToSandbox() add resources to sandbox in parallel, it issues
// runV API parallelly to send the NIC, Vols, and Containers to sandbox
func (p *XPod) addResourcesToSandbox() error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if !p.IsAlive() {
		p.Log(ERROR, "pod is not alive, can not add resources to sandbox")
		return errors.ErrPodNotAlive.WithArgs(p.Id())
	}

	p.Log(INFO, "adding resource to sandbox")
	future := utils.NewFutureSet()

	future.Add("addInterface", func() error {
		for _, inf := range p.interfaces {
			if err := inf.add(); err != nil {
				return err
			}
		}
		err := p.sandbox.AddRoute()
		if err != nil {
			p.Log(ERROR, "fail to add Route: %v", err)
		}
		return err
	})

	for iv, vol := range p.volumes {
		future.Add(iv, vol.add)
	}

	for ic, c := range p.containers {
		future.Add(ic, c.addToSandbox)
	}

	if p.services.size() != 0 {
		future.Add("serivce", p.services.apply)
	}

	if err := future.Wait(ProvisionTimeout); err != nil {
		p.Log(ERROR, "error during add resources to sandbox: %v", err)
		return err
	}
	return nil
}

func (p *XPod) startAll() error {
	p.Log(INFO, "start all containers")
	future := utils.NewFutureSet()

	for _, pre := range p.prestartExecs {
		p.Log(DEBUG, "run prestart exec %v", pre)
		_, stderr, err := p.sandbox.HyperstartExecSync(pre, nil)
		if err != nil {
			p.Log(ERROR, "failed to execute prestart command %v: %v [ %s", pre, err, string(stderr))
			return err
		}
	}

	for ic, c := range p.containers {
		future.Add(ic, c.start)
	}

	if err := future.Wait(ProvisionTimeout); err != nil {
		p.Log(ERROR, "error during start all containers: %v", err)
		return err
	}
	return nil
}

func (p *XPod) sandboxShareDir() string {
	if p.sandbox == nil {
		// the /dev/null is not a dir, then, can not create or open it
		return "/dev/null/no-such-dir"
	}
	return filepath.Join(hypervisor.BaseDir, p.sandbox.Id, hypervisor.ShareDirTag)
}

func (p *XPod) waitPodRun(activity string) error {
	p.statusLock.RLock()
	for {
		if p.status == S_POD_RUNNING || p.status == S_POD_PAUSED {
			p.statusLock.RUnlock()
			p.Log(DEBUG, "pod is running, proceed %s", activity)
			return nil
		}
		if p.status != S_POD_STARTING {
			p.statusLock.RUnlock()
			// only starting could transit to running, if not starting, that's mean failed
			p.Log(ERROR, "pod is not running, cannot %s", activity)
			return errors.ErrPodNotRunning.WithArgs(p.Id())
		}
		p.Log(TRACE, "wait for pod running")
		p.initCond.Wait()
	}
	// should never reach here
	p.statusLock.RUnlock()
	return errors.ErrorCodeCommon.WithArgs("reach unreachable code...")
}
