package daemon

import (
	"fmt"
	"os"
	"strings"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/sysinfo"
	"github.com/hyperhq/hyper/types"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	runvtypes "github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdInfo(job *engine.Job) error {
	cli := daemon.DockerCli
	sys, err := cli.SendCmdInfo("")
	if err != nil {
		return err
	}

	var num = 0
	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	for _, p := range daemon.PodList {
		num += len(p.Containers)
	}
	daemon.PodsMutex.RUnlock()
	glog.V(2).Infof("unlock read of PodList")
	v := &engine.Env{}
	v.Set("ID", daemon.ID)
	v.SetInt("Containers", num)
	v.SetInt("Images", sys.Images)
	v.Set("Driver", sys.Driver)
	v.SetJson("DriverStatus", sys.DriverStatus)
	v.Set("DockerRootDir", sys.DockerRootDir)
	v.Set("IndexServerAddress", sys.IndexServerAddress)
	v.Set("ExecutionDriver", daemon.Hypervisor)

	// Get system infomation
	meminfo, err := sysinfo.GetMemInfo()
	if err != nil {
		return err
	}
	osinfo, err := sysinfo.GetOSInfo()
	if err != nil {
		return err
	}
	v.SetInt64("MemTotal", int64(meminfo.MemTotal))
	v.SetInt64("Pods", daemon.GetPodNum())
	v.Set("Operating System", osinfo.PrettyName)
	if hostname, err := os.Hostname(); err == nil {
		v.SetJson("Name", hostname)
	}
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) CmdVersion(job *engine.Job) error {
	v := &engine.Env{}
	v.Set("ID", daemon.ID)
	v.Set("Version", fmt.Sprintf("\"%s\"", utils.VERSION))
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) CmdPodInfo(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not get Pod info without Pod ID")
	}
	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodsMutex.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")
	var (
		podId   string
		mypod   *hypervisor.Pod
		userpod *pod.UserPod
		ok      bool
		imageid string
	)
	if strings.Contains(job.Args[0], "pod-") {
		podId = job.Args[0]
		// We need to find the VM which running the POD
		mypod, ok = daemon.PodList[podId]
		if !ok {
			return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
		}
	} else {
		for _, p := range daemon.PodList {
			if p.Name == job.Args[0] {
				mypod = p
				break
			}
		}
		if mypod == nil {
			return fmt.Errorf("Can not get Pod info with pod name(%s)", job.Args[0])
		}
	}
	podData, err := daemon.GetPodByName(mypod.Id)
	if err == nil {
		if userpod, err = daemon.ProcessPodBytes(podData, mypod.Id); err != nil {
			return err
		}
	}
	// Construct the PodInfo JSON structure
	cStatus := []types.ContainerStatus{}
	containers := []types.Container{}
	for i, c := range mypod.Containers {
		ports := []types.ContainerPort{}
		envs := []types.EnvironmentVar{}
		vols := []types.VolumeMount{}
		jsonResponse, err := daemon.DockerCli.GetContainerInfo(c.Id)
		if err == nil {
			for _, e := range jsonResponse.Config.Env {
				envs = append(envs, types.EnvironmentVar{
					Env:   e[:strings.Index(e, "=")],
					Value: e[strings.Index(e, "=")+1:]})
			}
			imageid = jsonResponse.Image
		}
		for _, port := range userpod.Containers[i].Ports {
			ports = append(ports, types.ContainerPort{
				HostPort:      port.HostPort,
				ContainerPort: port.ContainerPort,
				Protocol:      port.Protocol})
		}
		for _, e := range userpod.Containers[i].Envs {
			envs = append(envs, types.EnvironmentVar{
				Env:   e.Env,
				Value: e.Value})
		}
		for _, v := range userpod.Containers[i].Volumes {
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
			Commands:        userpod.Containers[i].Command,
			Args:            []string{},
			Workdir:         userpod.Containers[i].Workdir,
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
			s.Running.StartedAt = mypod.StartedAt
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
			s.Terminated.StartedAt = mypod.StartedAt
			s.Terminated.FinishedAt = mypod.FinishedAt
		}
		cStatus = append(cStatus, s)
	}
	podVoumes := []types.PodVolume{}
	for _, v := range userpod.Volumes {
		podVoumes = append(podVoumes, types.PodVolume{
			Name:     v.Name,
			HostPath: v.Source,
			Driver:   v.Driver})
	}
	spec := types.PodSpec{
		Volumes:    podVoumes,
		Containers: containers,
	}
	podIPs := []string{}
	if mypod.Vm != "" {
		var vm *hypervisor.Vm = nil
		for _, m := range daemon.VmList {
			if mypod.Vm == m.Id {
				vm = m
				break
			}
		}
		if vm != nil {
			podIPs = mypod.GetPodIP(vm)
		}
	}
	status := types.PodStatus{
		Status:    cStatus,
		HostIP:    utils.GetHostIP(),
		PodIP:     podIPs,
		StartTime: mypod.StartedAt,
	}
	switch mypod.Status {
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

	data := types.PodInfo{
		Kind:       "Pod",
		ApiVersion: utils.APIVERSION,
		Vm:         mypod.Vm,
		Spec:       spec,
		Status:     status,
	}
	v := &engine.Env{}
	v.SetJson("data", data)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdContainerInfo(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not get Pod info without Pod ID")
	}
	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer daemon.PodsMutex.RUnlock()
	defer glog.V(2).Infof("unlock read of PodList")
	var (
		find    bool = false
		mypod   *hypervisor.Pod
		c       *hypervisor.Container
		i       int = 0
		imageid string
		userpod *pod.UserPod
		name    string = job.Args[0]
	)
	if name == "" {
		return fmt.Errorf("Null container name")
	}
	glog.Infof(name)
	for _, mypod = range daemon.PodList {
		for _, c = range mypod.Containers {
			glog.Infof(c.Name)
			if name[0] != '/' {
				if c.Name == "/"+name {
					find = true
					break
				}
			} else {
				if c.Name == name {
					find = true
					break
				}
			}
			if c.Id == name {
				find = true
				break
			}
		}
		if find == true {
			break
		}
	}
	if find == false {
		return fmt.Errorf("Can not find container by name(%s)", name)
	}
	podData, err := daemon.GetPodByName(mypod.Id)
	if err == nil {
		if userpod, err = daemon.ProcessPodBytes(podData, mypod.Id); err != nil {
			return err
		}
	}
	for k, v := range mypod.Containers {
		if v.Name == c.Name {
			i = k
			break
		}
	}

	ports := []types.ContainerPort{}
	envs := []types.EnvironmentVar{}
	vols := []types.VolumeMount{}
	jsonResponse, err := daemon.DockerCli.GetContainerInfo(c.Id)
	if err == nil {
		for _, e := range jsonResponse.Config.Env {
			envs = append(envs, types.EnvironmentVar{
				Env:   e[:strings.Index(e, "=")],
				Value: e[strings.Index(e, "=")+1:]})
		}
		imageid = jsonResponse.Image
	}
	for _, port := range userpod.Containers[i].Ports {
		ports = append(ports, types.ContainerPort{
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol})
	}
	for _, e := range userpod.Containers[i].Envs {
		envs = append(envs, types.EnvironmentVar{
			Env:   e.Env,
			Value: e.Value})
	}
	for _, v := range userpod.Containers[i].Volumes {
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
		s.Running.StartedAt = mypod.StartedAt
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
		s.Terminated.StartedAt = mypod.StartedAt
		s.Terminated.FinishedAt = mypod.FinishedAt
	}
	container := types.ContainerInfo{
		Name:            c.Name,
		ContainerID:     c.Id,
		Image:           c.Image,
		ImageID:         imageid,
		Commands:        userpod.Containers[i].Command,
		Args:            []string{},
		Workdir:         userpod.Containers[i].Workdir,
		Ports:           ports,
		Environment:     envs,
		Volume:          vols,
		ImagePullPolicy: "",
		Status:          s,
	}
	v := &engine.Env{}
	v.SetJson("data", container)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
