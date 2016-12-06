package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
)

func (daemon *Daemon) ExitCode(containerId, execId string) (int, error) {

	p, id, ok := daemon.PodList.GetByContainerIdOrName(containerId)
	if !ok {
		err := fmt.Errorf("cannot find container %s", containerId)
		glog.Error(err)
		return 255, err
	}

	glog.V(1).Infof("Get Exec Code for container %s", containerId)

	code, err := p.GetExitCode(id, execId)
	return int(code), err
}

func (daemon *Daemon) CreateExec(containerId, cmd string, terminal bool) (string, error) {

	p, id, ok := daemon.PodList.GetByContainerIdOrName(containerId)
	if !ok {
		err := fmt.Errorf("cannot find container %s", containerId)
		glog.Error(err)
		return "", err
	}

	glog.V(1).Infof("Create Exec for container %s", containerId)
	return p.CreateExec(id, cmd, terminal)
}

func (daemon *Daemon) StartExec(stdin io.ReadCloser, stdout io.WriteCloser, containerId, execId string) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(containerId)
	if !ok {
		err := fmt.Errorf("cannot find container %s", containerId)
		glog.Error(err)
		return err
	}

	glog.V(1).Infof("Start Exec for container %s", containerId)
	return p.StartExec(stdin, stdout, id, execId)
}
