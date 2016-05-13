package daemon

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	runvtypes "github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) GetPodInfo(podName string) (types.PodInfo, error) {
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
	cStatus := []*types.ContainerStatus{}
	containers := []*types.Container{}
	for i, c := range pod.status.Containers {
		ports := []*types.ContainerPort{}
		envs := []*types.EnvironmentVar{}
		vols := []*types.VolumeMount{}
		cmd := []string{}
		args := []string{}

		if len(pod.ctnStartInfo) > i {
			ci := pod.ctnStartInfo[i]
			for k, v := range ci.Envs {
				envs = append(envs, &types.EnvironmentVar{Env: k, Value: v})
			}
			imageid = c.Image
			cmd = ci.Cmd[:1]
			args = ci.Cmd[1:]
		}
		if len(cmd) == 0 {
			cmd = pod.spec.Containers[i].Command
		}
		for _, port := range pod.spec.Containers[i].Ports {
			ports = append(ports, &types.ContainerPort{
				HostPort:      int32(port.HostPort),
				ContainerPort: int32(port.ContainerPort),
				Protocol:      port.Protocol})
		}
		for _, e := range pod.spec.Containers[i].Envs {
			envs = append(envs, &types.EnvironmentVar{
				Env:   e.Env,
				Value: e.Value})
		}
		for _, v := range pod.spec.Containers[i].Volumes {
			vols = append(vols, &types.VolumeMount{
				Name:      v.Volume,
				MountPath: v.Path,
				ReadOnly:  v.ReadOnly})
		}
		container := types.Container{
			Name:            c.Name,
			ContainerID:     c.Id,
			Image:           pod.spec.Containers[i].Image,
			ImageID:         imageid,
			Commands:        cmd,
			Args:            args,
			WorkingDir:      pod.spec.Containers[i].Workdir,
			Ports:           ports,
			Env:             envs,
			VolumeMounts:    vols,
			Tty:             pod.spec.Containers[i].Tty,
			ImagePullPolicy: "",
		}
		containers = append(containers, &container)
		// Set ContainerStatus
		s := types.ContainerStatus{}
		s.Name = c.Name
		s.ContainerID = c.Id
		s.Waiting = &types.WaitingStatus{Reason: ""}
		s.Running = &types.RunningStatus{StartedAt: ""}
		s.Terminated = &types.TermStatus{}
		if c.Status == runvtypes.S_POD_CREATED {
			s.Waiting.Reason = "Pending"
			s.Phase = "pending"
		} else if c.Status == runvtypes.S_POD_RUNNING {
			s.Running.StartedAt = pod.status.StartedAt
			s.Phase = "running"
		} else { // S_POD_FAILED or S_POD_SUCCEEDED
			if c.Status == runvtypes.S_POD_FAILED {
				s.Terminated.ExitCode = int32(c.ExitCode)
				s.Terminated.Reason = "Failed"
				s.Phase = "failed"
			} else {
				s.Terminated.ExitCode = int32(c.ExitCode)
				s.Terminated.Reason = "Succeeded"
				s.Phase = "succeeded"
			}
			s.Terminated.StartedAt = pod.status.StartedAt
			s.Terminated.FinishedAt = pod.status.FinishedAt
		}
		cStatus = append(cStatus, &s)
	}
	podVoumes := []*types.PodVolume{}
	for _, v := range pod.spec.Volumes {
		podVoumes = append(podVoumes, &types.PodVolume{
			Name:   v.Name,
			Source: v.Source,
			Driver: v.Driver})
	}
	spec := types.PodSpec{
		Volumes:    podVoumes,
		Containers: containers,
		Labels:     pod.spec.Labels,
		Vcpu:       int32(pod.spec.Resource.Vcpu),
		Memory:     int32(pod.spec.Resource.Memory),
	}
	podIPs := []string{}
	if pod.status.Status == runvtypes.S_POD_RUNNING && pod.vm != nil {
		podIPs = pod.status.GetPodIP(pod.vm)
	}
	status := types.PodStatus{
		ContainerStatus: cStatus,
		HostIP:          utils.GetHostIP(),
		PodIP:           podIPs,
		StartTime:       pod.status.StartedAt,
		FinishTime:      pod.status.FinishedAt,
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
		Spec:       &spec,
		Status:     &status,
	}, nil
}

func (daemon *Daemon) GetPodStats(podId string) (interface{}, error) {
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

	var (
		pod     *Pod
		c       *hypervisor.Container
		i       int = 0
		imageid string
		ok      bool
		cmd     []string
		args    []string
	)
	if name == "" {
		return types.ContainerInfo{}, fmt.Errorf("Null container name")
	}
	glog.Infof(name)

	pod, i, ok = daemon.PodList.GetByContainerIdOrName(name)
	if !ok {
		return types.ContainerInfo{}, fmt.Errorf("Can not find container by name(%s)", name)
	}
	c = pod.status.Containers[i]

	ports := []*types.ContainerPort{}
	envs := []*types.EnvironmentVar{}
	vols := []*types.VolumeMount{}
	if len(pod.ctnStartInfo) > i {
		ci := pod.ctnStartInfo[i]
		for k, v := range ci.Envs {
			envs = append(envs, &types.EnvironmentVar{Env: k, Value: v})
		}
		imageid = c.Image
		cmd = ci.Cmd[:1]
		args = ci.Cmd[1:]
	}
	if len(cmd) == 0 {
		glog.Warning("length of commands in inspect result should not be zero")
		cmd = pod.spec.Containers[i].Command
	}
	for _, port := range pod.spec.Containers[i].Ports {
		ports = append(ports, &types.ContainerPort{
			HostPort:      int32(port.HostPort),
			ContainerPort: int32(port.ContainerPort),
			Protocol:      port.Protocol})
	}
	for _, e := range pod.spec.Containers[i].Envs {
		envs = append(envs, &types.EnvironmentVar{
			Env:   e.Env,
			Value: e.Value})
	}
	for _, v := range pod.spec.Containers[i].Volumes {
		vols = append(vols, &types.VolumeMount{
			Name:      v.Volume,
			MountPath: v.Path,
			ReadOnly:  v.ReadOnly})
	}
	s := types.ContainerStatus{}
	s.Name = c.Name
	s.ContainerID = c.Id
	s.Waiting = &types.WaitingStatus{Reason: ""}
	s.Running = &types.RunningStatus{StartedAt: ""}
	s.Terminated = &types.TermStatus{}
	if c.Status == runvtypes.S_POD_CREATED {
		s.Waiting.Reason = "Pending"
		s.Phase = "pending"
	} else if c.Status == runvtypes.S_POD_RUNNING {
		s.Running.StartedAt = pod.status.StartedAt
		s.Phase = "running"
	} else { // S_POD_FAILED or S_POD_SUCCEEDED
		if c.Status == runvtypes.S_POD_FAILED {
			s.Terminated.ExitCode = int32(c.ExitCode)
			s.Terminated.Reason = "Failed"
			s.Phase = "failed"
		} else {
			s.Terminated.ExitCode = int32(c.ExitCode)
			s.Terminated.Reason = "Succeeded"
			s.Phase = "succeeded"
		}
		s.Terminated.StartedAt = pod.status.StartedAt
		s.Terminated.FinishedAt = pod.status.FinishedAt
	}
	return types.ContainerInfo{
		Name:            c.Name,
		ContainerID:     c.Id,
		Image:           pod.spec.Containers[i].Image,
		ImageID:         imageid,
		Commands:        cmd,
		Args:            args,
		WorkingDir:      pod.spec.Containers[i].Workdir,
		Ports:           ports,
		Env:             envs,
		VolumeMounts:    vols,
		Tty:             pod.spec.Containers[i].Tty,
		ImagePullPolicy: "",

		PodID:  pod.Id,
		Status: &s,
	}, nil
}
