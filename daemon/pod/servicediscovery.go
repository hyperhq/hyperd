package pod

import (
	"fmt"
	"path"

	"github.com/hyperhq/hyperd/servicediscovery"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
)

func (p *XPod) ParseServiceDiscovery(spec *apitypes.UserPod) *apitypes.UserContainer {
	var serviceType string = "service-discovery"

	if len(spec.Services) == 0 || spec.Type == serviceType {
		return nil
	}

	spec.Type = serviceType

	return p.generateServiceContainer(spec.Services)
}

func (p *XPod) generateServiceContainer(srvs []*apitypes.UserService) *apitypes.UserContainer {
	var serviceDir string = path.Join(utils.HYPER_ROOT, "services", p.Id())

	/* PrepareServices will check service volume */
	serviceVolume := &apitypes.UserVolume{
		Name:   "service-volume",
		Source: serviceDir,
		Format: "vfs",
		Fstype: "dir",
	}

	serviceVolRef := &apitypes.UserVolumeReference{
		Volume:   "service-volume",
		Path:     servicediscovery.ServiceVolume,
		ReadOnly: false,
		Detail:   serviceVolume,
	}

	return &apitypes.UserContainer{
		Name:       ServiceDiscoveryContainerName(p.Id()),
		Image:      servicediscovery.ServiceImage,
		Command:    []string{"haproxy", "-D", "-f", "/usr/local/etc/haproxy/haproxy.cfg", "-p", "/var/run/haproxy.pid"},
		Volumes:    []*apitypes.UserVolumeReference{serviceVolRef},
		Type:       apitypes.UserContainer_SERVICE,
		StopSignal: "KILL",
	}
}

func ServiceDiscoveryContainerName(podName string) string {
	return podName + "-" + utils.RandStr(10, "alpha") + "-service-discovery"
}

func (p *XPod) GetServices() ([]*apitypes.UserService, error) {
	return p.services, nil
}

func (p *XPod) NewServiceContainer(srvs []*apitypes.UserService) error {
	if !p.IsRunning() {
		err := fmt.Errorf("unable to add service container when pod is not running")
		p.Log(ERROR, err)
		return err
	}

	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	p.Log(INFO, "create new service container")

	p.services = srvs

	if err := p.setupServiceInf(); err != nil {
		p.Log(ERROR, "failed to create service interfaces: %v", err)
		return err
	}
	sc := p.generateServiceContainer(srvs)
	cid, err := p.doContainerCreate(sc)
	if err != nil {
		p.Log(ERROR, "failed to create service container")
		return err
	}
	err = p.ContainerStart(cid)
	if err != nil {
		p.Log(ERROR, "failed to start service container")
		return err
	}

	return nil
}

func (p *XPod) UpdateService(srvs []*apitypes.UserService) error {
	if p.globalSpec.Type != "service-discovery" {
		p.globalSpec.Type = "service-discovery"
		p.Log(INFO, "change pod type to service discovery")
		return p.NewServiceContainer(srvs)
	}
	p.Log(INFO, "update service %v", srvs)
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if p.IsRunning() {
		sc := p.getServiceContainer()
		p.Log(DEBUG, "apply service to service container")
		if err := servicediscovery.ApplyServices(p.sandbox, sc, srvs); err != nil {
			p.Log(ERROR, "failed to update services %#v: %v", srvs, err)
			return err
		}
	}
	p.services = srvs
	return nil
}

func (p *XPod) AddService(srvs []*apitypes.UserService) error {
	if p.globalSpec.Type != "service-discovery" {
		p.globalSpec.Type = "service-discovery"
		p.Log(INFO, "change pod type to service discovery")
		return p.NewServiceContainer(srvs)
	}
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	target := append(p.services, srvs...)

	if p.IsRunning() {
		sc := p.getServiceContainer()
		if err := servicediscovery.ApplyServices(p.sandbox, sc, target); err != nil {
			p.Log(ERROR, "failed to update services %#v: %v", target, err)
			return err
		}
	}
	p.services = target
	return nil
}

func (p *XPod) DeleteService(srvs []*apitypes.UserService) error {
	if p.globalSpec.Type != "service-discovery" {
		err := fmt.Errorf("pod does not support service discovery")
		p.Log(ERROR, err)
		return err
	}
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	tbd := make(map[struct {
		IP   string
		Port int32
	}]bool, len(srvs))
	for _, srv := range srvs {
		tbd[struct {
			IP   string
			Port int32
		}{srv.ServiceIP, srv.ServicePort}] = true
	}
	target := make([]*apitypes.UserService, 0, len(p.services))
	for _, srv := range p.services {
		if tbd[struct {
			IP   string
			Port int32
		}{srv.ServiceIP, srv.ServicePort}] {
			p.Log(TRACE, "remove service %#v", srv)
			continue
		}
		target = append(target, srv)
	}

	if p.IsRunning() {
		sc := p.getServiceContainer()
		if err := servicediscovery.ApplyServices(p.sandbox, sc, target); err != nil {
			p.Log(ERROR, "failed to update services %#v: %v", target, err)
			return err
		}
	}
	p.services = target
	return nil
}

func (p *XPod) getServiceContainer() string {
	if p.globalSpec.Type != "service-discovery" {
		return ""
	}
	for _, c := range p.containers {
		if c.spec.Type == apitypes.UserContainer_SERVICE {
			return c.Id()
		}
	}
	return ""
}
