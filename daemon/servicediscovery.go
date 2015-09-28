package daemon

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/servicediscovery"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) AddService(job *engine.Job) error {
	var srvs []pod.UserService

	podId := job.Args[0]
	data := job.Args[1]

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

	return servicediscovery.ApplyServices(vm, container, services)
}

func (daemon *Daemon) UpdateService(job *engine.Job) error {
	var srv []pod.UserService

	podId := job.Args[0]
	data := job.Args[1]

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(data), &srv)
	if err != nil {
		return err
	}

	return servicediscovery.ApplyServices(vm, container, srv)
}

func (daemon *Daemon) DeleteService(job *engine.Job) error {
	var srvs []pod.UserService
	var services []pod.UserService
	var services2 []pod.UserService
	var found int = 0

	podId := job.Args[0]
	data := job.Args[1]

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

	return servicediscovery.ApplyServices(vm, container, services2)
}

func (daemon *Daemon) GetServices(job *engine.Job) error {
	podId := job.Args[0]

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err := servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	v := &engine.Env{}
	v.SetJson("data", services)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) GetServiceContainerInfo(podId string) (*hypervisor.Vm, string, error) {
	daemon.PodsMutex.RLock()
	mypod, ok := daemon.PodList[podId]
	if !ok {
		daemon.PodsMutex.RUnlock()
		return nil, "", fmt.Errorf("Cannot find Pod %s", podId)
	}

	if mypod.Type != "service-discovery" || len(mypod.Containers) <= 1 {
		daemon.PodsMutex.RUnlock()
		return nil, "", fmt.Errorf("Pod %s doesn't have services discovery", podId)
	}

	container := mypod.Containers[0].Id
	vmId := mypod.Vm
	glog.V(1).Infof("Get container id is %s, vm %s", container, vmId)
	daemon.PodsMutex.RUnlock()

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return nil, "", fmt.Errorf("Can find VM whose Id is %s!", vmId)
	}

	return vm, container, nil
}

func (daemon *Daemon) ProcessPodBytes(body []byte, podId string) (*pod.UserPod, error) {
	var containers []pod.UserContainer
	var serviceDir string = path.Join(utils.HYPER_ROOT, "services", podId)

	userPod, err := pod.ProcessPodBytes(body)
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return nil, err
	}

	if len(userPod.Services) == 0 {
		return userPod, nil
	}

	userPod.Type = "service-discovery"
	serviceContainer := pod.UserContainer{
		Name:    userPod.Name + "-service-discovery",
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

	userPod.Volumes = append(userPod.Volumes, serviceVolume)

	serviceContainer.Volumes = append(serviceContainer.Volumes, serviceVolRef)

	containers = append(containers, serviceContainer)

	for _, c := range userPod.Containers {
		containers = append(containers, c)
	}

	userPod.Containers = containers

	return userPod, nil
}
