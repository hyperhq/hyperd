package daemon

import (
	"fmt"

	"github.com/golang/glog"
)

func (daemon Daemon) PausePod(podId string) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
	}

	return p.Pause()
}

func (daemon Daemon) PauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	p, _, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		err := fmt.Errorf("cannot get container %s to pause", container)
		glog.Error(err)
		return err
	}

	return p.Pause()
}

func (daemon *Daemon) UnpausePod(podId string) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
	}

	return p.UnPause()
}

func (daemon *Daemon) UnpauseContainer(container string) error {
	glog.V(1).Infof("Get container id is %s", container)
	p, _, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		err := fmt.Errorf("cannot get container %s to pause", container)
		glog.Error(err)
		return err
	}

	return p.UnPause()
}
