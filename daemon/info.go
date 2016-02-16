package daemon

import (
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/version"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/types"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	runvtypes "github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) GetPodInfo(podName string) (types.PodInfo, error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodList.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")
	var (
		pod     *Pod
		ok      bool
		imageid string
	)
	if strings.Contains(podName, "pod-") {
		pod, ok = daemon.PodList.Get(podName)
		if !ok {
			return types.PodInfo{}, fmt.Errorf("Can not get Pod info with pod ID(%s)", podName)
		}
	} else {
		pod = daemon.PodList.GetByName(podName)
		if pod == nil {
			return types.PodInfo{}, fmt.Errorf("Can not get Pod info with pod name(%s)", podName)
		}
	}

	// Construct the PodInfo JSON structure
	cStatus := []types.ContainerStatus{}
	containers := []types.Container{}
	for i, c := range pod.status.Containers {
		ports := []types.ContainerPort{}
		envs := []types.EnvironmentVar{}
		vols := []types.VolumeMount{}
		Response, err := daemon.Daemon.ContainerInspect(c.Id, false, version.Version("1.21"))
		if err == nil {
			var jsonResponse *dockertypes.ContainerJSON
			jsonResponse, _ = Response.(*dockertypes.ContainerJSON)

			for _, e := range jsonResponse.Config.Env {
				envs = append(envs, types.EnvironmentVar{
					Env:   e[:strings.Index(e, "=")],
					Value: e[strings.Index(e, "=")+1:]})
			}
			imageid = jsonResponse.Image
		}
		for _, port := range pod.spec.Containers[i].Ports {
			ports = append(ports, types.ContainerPort{
				HostPort:      port.HostPort,
				ContainerPort: port.ContainerPort,
				Protocol:      port.Protocol})
		}
		for _, e := range pod.spec.Containers[i].Envs {
			envs = append(envs, types.EnvironmentVar{
				Env:   e.Env,
				Value: e.Value})
		}
		for _, v := range pod.spec.Containers[i].Volumes {
			vols = append(vols, types.VolumeMount{
				Name:      v.Volume,
				MountPath: v.Path,
				ReadOnly:  v.ReadOnly})
		}
		container := types.Container{
			Name:            c.Name,
			ContainerID:     c.Id,
			Image:           c.Image,
			ImageID:         imageid,
			Commands:        pod.spec.Containers[i].Command,
			Args:            []string{},
			Workdir:         pod.spec.Containers[i].Workdir,
			Ports:           ports,
			Environment:     envs,
			Volume:          vols,
			ImagePullPolicy: "",
		}
		containers = append(containers, container)
		// Set ContainerStatus
		s := types.ContainerStatus{}
		s.Name = c.Name
		s.ContainerID = c.Id
		s.Waiting = types.WaitingStatus{Reason: ""}
		s.Running = types.RunningStatus{StartedAt: ""}
		s.Terminated = types.TermStatus{}
		if c.Status == runvtypes.S_POD_CREATED {
			s.Waiting.Reason = "Pending"
			s.Phase = "pending"
		} else if c.Status == runvtypes.S_POD_RUNNING {
			s.Running.StartedAt = pod.status.StartedAt
			s.Phase = "running"
		} else { // S_POD_FAILED or S_POD_SUCCEEDED
			if c.Status == runvtypes.S_POD_FAILED {
				s.Terminated.ExitCode = c.ExitCode
				s.Terminated.Reason = "Failed"
				s.Phase = "failed"
			} else {
				s.Terminated.ExitCode = c.ExitCode
				s.Terminated.Reason = "Succeeded"
				s.Phase = "succeeded"
			}
			s.Terminated.StartedAt = pod.status.StartedAt
			s.Terminated.FinishedAt = pod.status.FinishedAt
		}
		cStatus = append(cStatus, s)
	}
	podVoumes := []types.PodVolume{}
	for _, v := range pod.spec.Volumes {
		podVoumes = append(podVoumes, types.PodVolume{
			Name:     v.Name,
			HostPath: v.Source,
			Driver:   v.Driver})
	}
	spec := types.PodSpec{
		Volumes:    podVoumes,
		Containers: containers,
		Labels:     pod.spec.Labels,
		Vcpu:       pod.spec.Resource.Vcpu,
		Memory:     pod.spec.Resource.Memory,
	}
	podIPs := []string{}
	if pod.vm != nil {
		podIPs = pod.status.GetPodIP(pod.vm)
	}
	status := types.PodStatus{
		Status:    cStatus,
		HostIP:    utils.GetHostIP(),
		PodIP:     podIPs,
		StartTime: pod.status.StartedAt,
	}
	switch pod.status.Status {
	case runvtypes.S_POD_CREATED:
		status.Phase = "Pending"
		break
	case runvtypes.S_POD_RUNNING:
		status.Phase = "Running"
		break
	case runvtypes.S_POD_SUCCEEDED:
		status.Phase = "Succeeded"
		break
	case runvtypes.S_POD_FAILED:
		status.Phase = "Failed"
		break
	}

	return types.PodInfo{
		Kind:       "Pod",
		ApiVersion: utils.APIVERSION,
		Vm:         pod.status.Vm,
		Spec:       spec,
		Status:     status,
	}, nil
}

func (daemon *Daemon) GetPodStats(podId string) (interface{}, error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodList.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")
	var (
		pod *Pod
		ok  bool
	)
	if strings.Contains(podId, "pod-") {
		pod, ok = daemon.PodList.Get(podId)
		if !ok {
			return nil, fmt.Errorf("Can not get Pod stats with pod ID(%s)", podId)
		}
	} else {
		pod = daemon.PodList.GetByName(podId)
		if pod == nil {
			return nil, fmt.Errorf("Can not get Pod stats with pod name(%s)", podId)
		}
	}

	if pod.vm == nil || pod.status.Status != runvtypes.S_POD_RUNNING {
		return nil, fmt.Errorf("Can not get pod stats for non-running pod (%s)", podId)
	}

	response := pod.vm.Stats()
	if response.Data == nil {
		return nil, fmt.Errorf("Stats for pod %s is nil", podId)
	}

	return response.Data, nil
}

func (daemon *Daemon) GetContainerInfo(name string) (types.ContainerInfo, error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodList.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")

	var (
		pod     *Pod
		c       *hypervisor.Container
		i       int = 0
		imageid string
	)
	if name == "" {
		return types.ContainerInfo{}, fmt.Errorf("Null container name")
	}
	glog.Infof(name)
	wslash := name
	if name[0] != '/' {
		wslash = "/" + name
	}
	pod = daemon.PodList.Find(func(p *Pod) bool {
		for i, c = range p.status.Containers {
			if c.Name == wslash || c.Id == name {
				return true
			}
		}
		return false
	})
	if pod == nil {
		return types.ContainerInfo{}, fmt.Errorf("Can not find container by name(%s)", name)
	}

	ports := []types.ContainerPort{}
	envs := []types.EnvironmentVar{}
	vols := []types.VolumeMount{}
	rsp, err := daemon.Daemon.ContainerInspect(c.Id, false, version.Version("1.21"))
	if err == nil {
		var jsonResponse *dockertypes.ContainerJSON
		jsonResponse, _ = rsp.(*dockertypes.ContainerJSON)

		for _, e := range jsonResponse.Config.Env {
			envs = append(envs, types.EnvironmentVar{
				Env:   e[:strings.Index(e, "=")],
				Value: e[strings.Index(e, "=")+1:]})
		}
		imageid = jsonResponse.Image
	}
	for _, port := range pod.spec.Containers[i].Ports {
		ports = append(ports, types.ContainerPort{
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol})
	}
	for _, e := range pod.spec.Containers[i].Envs {
		envs = append(envs, types.EnvironmentVar{
			Env:   e.Env,
			Value: e.Value})
	}
	for _, v := range pod.spec.Containers[i].Volumes {
		vols = append(vols, types.VolumeMount{
			Name:      v.Volume,
			MountPath: v.Path,
			ReadOnly:  v.ReadOnly})
	}
	s := types.ContainerStatus{}
	s.Name = c.Name
	s.ContainerID = c.Id
	s.Waiting = types.WaitingStatus{Reason: ""}
	s.Running = types.RunningStatus{StartedAt: ""}
	s.Terminated = types.TermStatus{}
	if c.Status == runvtypes.S_POD_CREATED {
		s.Waiting.Reason = "Pending"
		s.Phase = "pending"
	} else if c.Status == runvtypes.S_POD_RUNNING {
		s.Running.StartedAt = pod.status.StartedAt
		s.Phase = "running"
	} else { // S_POD_FAILED or S_POD_SUCCEEDED
		if c.Status == runvtypes.S_POD_FAILED {
			s.Terminated.ExitCode = c.ExitCode
			s.Terminated.Reason = "Failed"
			s.Phase = "failed"
		} else {
			s.Terminated.ExitCode = c.ExitCode
			s.Terminated.Reason = "Succeeded"
			s.Phase = "succeeded"
		}
		s.Terminated.StartedAt = pod.status.StartedAt
		s.Terminated.FinishedAt = pod.status.FinishedAt
	}
	return types.ContainerInfo{
		Name:            c.Name,
		ContainerID:     c.Id,
		PodID:           pod.id,
		Image:           c.Image,
		ImageID:         imageid,
		Commands:        pod.spec.Containers[i].Command,
		Args:            []string{},
		Workdir:         pod.spec.Containers[i].Workdir,
		Ports:           ports,
		Environment:     envs,
		Volume:          vols,
		ImagePullPolicy: "",
		Status:          s,
	}, nil
}
