package daemon

import (
	"fmt"

	"github.com/golang/glog"
)

func (daemon *Daemon) StopPod(podId string) (int, string, error) {
	glog.Infof("Prepare to stop the POD: %s", podId)
	// find the vm id which running POD, and stop it
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		glog.Errorf("Can not find pod(%s)", podId)
		return -1, "", fmt.Errorf("Can not find pod(%s)", podId)
	}

	err := p.Stop(5)
	if err != nil {
		glog.Error(err)

		p.ForceQuit()
	}

	return 0, "", nil
}

func (daemon *Daemon) StopContainer(container string, graceful int) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		return fmt.Errorf("can not found container %s", container)
	}

	return p.StopContainer(id, graceful)
}
