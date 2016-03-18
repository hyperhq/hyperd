package daemon

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/servicediscovery"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (daemon *Daemon) AddService(podId, data string) error {
	var srvs []pod.UserService

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err := servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	json.Unmarshal([]byte(data), &srvs)
	if err != nil {
		return err
	}

	for _, s := range srvs {
		services = append(services, s)
	}

	if err := servicediscovery.ApplyServices(vm, container, services); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) UpdateService(podId, data string) error {
	var srv []pod.UserService

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(data), &srv)
	if err != nil {
		return err
	}

	if err := servicediscovery.ApplyServices(vm, container, srv); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) DeleteService(podId, data string) error {
	var srvs []pod.UserService
	var services []pod.UserService
	var services2 []pod.UserService
	var found int = 0

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err = servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(data), &srvs)
	if err != nil {
		return err
	}

	for _, s := range services {
		shouldRemain := true
		for _, srv := range srvs {
			if s.ServiceIP == srv.ServiceIP && s.ServicePort == srv.ServicePort {
				shouldRemain = false
				found = 1
				break
			}
		}

		if shouldRemain {
			services2 = append(services2, s)
		}
	}

	if found == 0 {
		return fmt.Errorf("Pod %s doesn't use this service", podId)
	}

	if err := servicediscovery.ApplyServices(vm, container, services2); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) GetServices(podId string) ([]pod.UserService, error) {
	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return nil, err
	}

	services, err := servicediscovery.GetServices(vm, container)
	if err != nil {
		return nil, err
	}

	return services, nil
}

func (daemon *Daemon) GetServiceContainerInfo(podId string) (*hypervisor.Vm, string, error) {
	daemon.PodList.RLock()
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		daemon.PodList.RUnlock()
		return nil, "", fmt.Errorf("Cannot find Pod %s", podId)
	}

	if pod.status.Type != "service-discovery" || len(pod.status.Containers) <= 1 {
		daemon.PodList.RUnlock()
		return nil, "", fmt.Errorf("Pod %s doesn't have services discovery", podId)
	}

	container := pod.status.Containers[0].Id
	glog.V(1).Infof("Get container id is %s", container)
	daemon.PodList.RUnlock()

	if pod.vm == nil {
		return nil, "", fmt.Errorf("Can find VM for %s!", podId)
	}

	return pod.vm, container, nil
}

func ParseServiceDiscovery(id string, spec *pod.UserPod) error {
	var containers []pod.UserContainer
	var serviceDir string = path.Join(utils.HYPER_ROOT, "services", id)

	if len(spec.Services) == 0 {
		return nil
	}

	spec.Type = "service-discovery"
	serviceContainer := pod.UserContainer{
		Name:    ServiceDiscoveryContainerName(spec.Name),
		Image:   servicediscovery.ServiceImage,
		Command: []string{"haproxy", "-D", "-f", "/usr/local/etc/haproxy/haproxy.cfg", "-p", "/var/run/haproxy.pid"},
	}

	serviceVolRef := pod.UserVolumeReference{
		Volume:   "service-volume",
		Path:     servicediscovery.ServiceVolume,
		ReadOnly: false,
	}

	/* PrepareServices will check service volume */
	serviceVolume := pod.UserVolume{
		Name:   "service-volume",
		Source: serviceDir,
		Driver: "vfs",
	}

	spec.Volumes = append(spec.Volumes, serviceVolume)

	serviceContainer.Volumes = append(serviceContainer.Volumes, serviceVolRef)

	containers = append(containers, serviceContainer)

	for _, c := range spec.Containers {
		containers = append(containers, c)
	}

	spec.Containers = containers

	return nil
}

func ServiceDiscoveryContainerName(podName string) string {
	return podName + "-service-discovery"
}
