package daemon

import (
	"fmt"

	"github.com/golang/glog"
)

const (
	E_NOT_FOUND       = -2
	E_UNDER_OPERATION = -1
	E_OK              = 0
)

func (daemon *Daemon) RemovePod(podId string) (int, string, error) {
	var (
		code  = E_OK
		cause = ""
		err   error
	)

	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return E_NOT_FOUND, "", fmt.Errorf("Can not find that Pod(%s)", podId)
	}

	daemon.PodList.Release(podId)

	if p.IsAlive() {
		glog.V(1).Infof("remove pod %s, stop it firstly", podId)
		p.Stop(5)
	}

	p.Remove(true)

	return code, cause, err
}

func (daemon *Daemon) RemoveContainer(nameOrId string) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(nameOrId)
	if !ok {
		err := fmt.Errorf("cannot find container %s", nameOrId)
		glog.Error(err)
		return err
	}

	return p.RemoveContainer(id)
}
