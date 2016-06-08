package daemon

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
)

func (daemon *Daemon) KillContainer(name string, sig int64) error {
	p, idx, ok := daemon.PodList.GetByContainerIdOrName(name)
	if !ok {
		return fmt.Errorf("can not find container %s", name)
	}

	container := p.status.Containers[idx].Id
	glog.V(1).Infof("found container %s to kill, signal %d", container, sig)

	return p.vm.KillContainer(container, syscall.Signal(sig))
}

func (daemon *Daemon) KillPodContainers(podName, container string, sig int64) error {
	p, ok := daemon.PodList.GetByName(podName)
	if !ok {
		return fmt.Errorf("can not find pod %s", podName)
	}

	var err error = nil
	all := (container == "")
	shot := false
	for i := range p.status.Containers {
		if all || p.status.Containers[i].Id == container {
			glog.V(1).Infof("send signal %d to container %s", sig, container)
			e := p.vm.KillContainer(p.status.Containers[i].Id, syscall.Signal(sig))
			if e != nil {
				err = e
			}
			shot = true
		}
	}
	if !shot {
		return fmt.Errorf("can not find container %s", container)
	}
	return err
}
