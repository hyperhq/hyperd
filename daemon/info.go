package daemon

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/hyperhq/hyperd/daemon/pod"
	"github.com/hyperhq/hyperd/types"
)

func (daemon *Daemon) GetPodInfo(podName string) (*types.PodInfo, error) {
	var (
		p  *pod.XPod
		ok bool
	)
	p, ok = daemon.PodList.Get(podName)
	if !ok {
		return &types.PodInfo{}, fmt.Errorf("Can not get Pod info with pod ID(%s)", podName)
	}

	return p.Info()
}

func (daemon *Daemon) GetPodStats(podId string) (interface{}, error) {
	var (
		p  *pod.XPod
		ok bool
	)
	p, ok = daemon.PodList.Get(podId)
	if !ok {
		return nil, fmt.Errorf("Can not get Pod stats with pod ID(%s)", podId)
	}

	if !p.IsRunning() {
		return nil, fmt.Errorf("Can not get pod stats for non-running pod (%s)", podId)
	}

	response := p.Stats()
	if response == nil || response.Data == nil {
		return nil, fmt.Errorf("Stats for pod %s is nil", podId)
	}

	return response.Data, nil
}

func (daemon *Daemon) GetContainerInfo(name string) (*types.ContainerInfo, error) {
	if name == "" {
		return &types.ContainerInfo{}, fmt.Errorf("Empty container name")
	}
	glog.V(3).Infof("GetContainerInfo of %s", name)

	p, id, ok := daemon.PodList.GetByContainerIdOrName(name)
	if !ok {
		return &types.ContainerInfo{}, fmt.Errorf("Can not find container by name(%s)", name)
	}

	return p.ContainerInfo(id)
}
