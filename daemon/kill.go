package daemon

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
)

func (daemon *Daemon) KillContainer(name string, sig int64) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(name)
	if !ok {
		return fmt.Errorf("can not find container %s", name)
	}

	glog.V(1).Infof("found container %s to kill, signal %d", name, sig)
	if p.IsContainerRunning(id) {
		return p.KillContainer(id, sig)
	}

	glog.V(1).Infof("container %s not in alive status, ignore kill", name)
	return nil
}

func (daemon *Daemon) KillPodContainers(podName, container string, sig int64) error {
	var err error

	p, ok := daemon.PodList.Get(podName)
	if !ok {
		err = fmt.Errorf("can not find pod %s", podName)
		glog.Error(err)
		return err
	}

	containers := []string{}
	if container != "" {
		cid, ok := p.ContainerName2Id(container)
		if !ok {
			err = fmt.Errorf("can not get container %s in pod %s", container, podName)
			glog.Error(err)
			return err
		}
		containers = append(containers, cid)
	} else {
		if syscall.Signal(sig) == syscall.SIGKILL {
			p.ForceQuit()
			glog.Infof("force kill sandbox for pod %s", podName)
			return nil
		}
		containers = p.ContainerIds()
	}

	for _, cid := range containers {
		if p.IsContainerRunning(cid) {
			e := p.KillContainer(cid, sig)
			if e != nil {
				glog.Error(e)
				err = e
			}
		}
	}
	return err
}
