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
		pod, ok = daemon.PodList.GetByName(podName)
		if !ok {
			return types.PodInfo{}, fmt.Errorf("Can not get Pod info with pod name(%s)", podName)
		}
	}

	cStatus := []*types.ContainerStatus{}
	containers := []*types.Container{}
	for i, c := range pod.PodStatus.Containers {
		ports := []*types.ContainerPort{}
		envs := []*types.EnvironmentVar{}
		vols := []*types.VolumeMount{}
		cmd := []string{}
		args := []string{}

		if len(pod.containers) > i {
			ci := pod.containers[i]
			envs = ci.ApiContainer.Env
			imageid = c.Image
			cmd = ci.ApiContainer.Commands
			args = ci.ApiContainer.Args
		}
		if len(cmd) == 0 {
			cmd = pod.Spec.Containers[i].Command
		}
		for _, port := range pod.Spec.Containers[i].Ports {
			ports = append(ports, &types.ContainerPort{
				HostPort:      int32(port.HostPort),
				ContainerPort: int32(port.ContainerPort),
				Protocol:      port.Protocol})
		}
		for _, e := range pod.Spec.Containers[i].Envs {
			envs = append(envs, &types.EnvironmentVar{
				Env:   e.Env,
				Value: e.Value})
		}
		for _, v := range pod.Spec.Containers[i].Volumes {
			vols = append(vols, &types.VolumeMount{
				Name:      v.Volume,
				MountPath: v.Path,
				ReadOnly:  v.ReadOnly})
		}

		container := types.Container{
			Name:            c.Name,
			ContainerID:     c.Id,
			Image:           pod.Spec.Containers[i].Image,
			ImageID:         imageid,
			Commands:        cmd,
			Args:            args,
			WorkingDir:      pod.Spec.Containers[i].Workdir,
			Labels:          pod.Spec.Containers[i].Labels,
			Ports:           ports,
			Env:             envs,
			VolumeMounts:    vols,
			Tty:             pod.Spec.Containers[i].Tty,
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
			s.Running.StartedAt = pod.PodStatus.StartedAt
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
			s.Terminated.StartedAt = pod.PodStatus.StartedAt
			s.Terminated.FinishedAt = pod.PodStatus.FinishedAt
		}
		cStatus = append(cStatus, &s)
	}

	podVoumes := []*types.PodVolume{}
	for _, v := range pod.Spec.Volumes {
		podVoumes = append(podVoumes, &types.PodVolume{
			Name:   v.Name,
			Source: v.Source,
			Driver: v.Driver})
	}

	spec := types.PodSpec{
		Volumes:    podVoumes,
		Containers: containers,
		Labels:     pod.Spec.Labels,
		Vcpu:       int32(pod.Spec.Resource.Vcpu),
		Memory:     int32(pod.Spec.Resource.Memory),
	}

	podIPs := []string{}
	if pod.PodStatus.Status == runvtypes.S_POD_RUNNING && pod.VM != nil {
		podIPs = pod.PodStatus.GetPodIP(pod.VM)
	}

	status := types.PodStatus{
		ContainerStatus: cStatus,
		HostIP:          utils.GetHostIP(),
		PodIP:           podIPs,
		StartTime:       pod.PodStatus.StartedAt,
		FinishTime:      pod.PodStatus.FinishedAt,
	}
	switch pod.PodStatus.Status {
	case runvtypes.S_POD_CREATED:
		status.Phase = "Pending"
		break
	case runvtypes.S_POD_RUNNING:
		status.Phase = "Running"
		break
	case runvtypes.S_POD_PAUSED:
		status.Phase = "Paused"
		break
	case runvtypes.S_POD_SUCCEEDED:
		status.Phase = "Succeeded"
		break
	case runvtypes.S_POD_FAILED:
		status.Phase = "Failed"
		break
	}

	return types.PodInfo{
		PodID:      pod.Id,
		PodName:    pod.Spec.Name,
		Kind:       "Pod",
		CreatedAt:  pod.CreatedAt,
		ApiVersion: utils.APIVERSION,
		Vm:         pod.PodStatus.Vm,
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
		pod, ok = daemon.PodList.GetByName(podId)
		if !ok {
			return nil, fmt.Errorf("Can not get Pod stats with pod name(%s)", podId)
		}
	}

	if pod.VM == nil || pod.PodStatus.Status != runvtypes.S_POD_RUNNING {
		return nil, fmt.Errorf("Can not get pod stats for non-running pod (%s)", podId)
	}

	response := pod.VM.Stats()
	if response.Data == nil {
		return nil, fmt.Errorf("Stats for pod %s is nil", podId)
	}

	return response.Data, nil
}

func (daemon *Daemon) GetContainerInfo(name string) (types.ContainerInfo, error) {
	var (
		pod       *Pod
		c         *hypervisor.ContainerStatus
		i         int   = 0
		createdAt int64 = 0
		imageid   string
		ok        bool
		cmd       []string
		args      []string
	)
	if name == "" {
		return types.ContainerInfo{}, fmt.Errorf("Null container name")
	}
	glog.Infof(name)

	pod, i, ok = daemon.PodList.GetByContainerIdOrName(name)
	if !ok {
		return types.ContainerInfo{}, fmt.Errorf("Can not find container by name(%s)", name)
	}
	c = pod.PodStatus.Containers[i]

	ports := []*types.ContainerPort{}
	envs := []*types.EnvironmentVar{}
	vols := []*types.VolumeMount{}
	if len(pod.containers) > i {
		ci := pod.containers[i]
		envs = ci.ApiContainer.Env
		imageid = c.Image
		cmd = ci.ApiContainer.Commands
		args = ci.ApiContainer.Args
		createdAt = ci.CreatedAt
	}
	if len(cmd) == 0 {
		glog.Warning("length of commands in inspect result should not be zero")
		cmd = pod.Spec.Containers[i].Command
	}
	for _, port := range pod.Spec.Containers[i].Ports {
		ports = append(ports, &types.ContainerPort{
			HostPort:      int32(port.HostPort),
			ContainerPort: int32(port.ContainerPort),
			Protocol:      port.Protocol})
	}
	for _, e := range pod.Spec.Containers[i].Envs {
		envs = append(envs, &types.EnvironmentVar{
			Env:   e.Env,
			Value: e.Value})
	}
	for _, v := range pod.Spec.Containers[i].Volumes {
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
		s.Running.StartedAt = pod.PodStatus.StartedAt
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
		s.Terminated.StartedAt = pod.PodStatus.StartedAt
		s.Terminated.FinishedAt = pod.PodStatus.FinishedAt
	}
	return types.ContainerInfo{
		Container: &types.Container{
			Name:            c.Name,
			ContainerID:     c.Id,
			Image:           pod.Spec.Containers[i].Image,
			ImageID:         imageid,
			Commands:        cmd,
			Args:            args,
			WorkingDir:      pod.Spec.Containers[i].Workdir,
			Labels:          pod.Spec.Containers[i].Labels,
			Ports:           ports,
			Env:             envs,
			VolumeMounts:    vols,
			Tty:             pod.Spec.Containers[i].Tty,
			ImagePullPolicy: "",
		},
		CreatedAt: createdAt,
		PodID:     pod.Id,
		Status:    &s,
	}, nil
}
