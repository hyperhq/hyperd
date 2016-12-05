package daemon

import (
	"fmt"
	"github.com/golang/glog"
)

func (daemon *Daemon) TtyResize(containerId, execId string, h, w int) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(containerId)
	if !ok {
		err := fmt.Errorf("cannot find container %s", containerId)
		glog.Error(err)
		return err
	}

	err := p.TtyResize(id, execId, h, w)
	if err != nil {
		return err
	}

	glog.V(1).Infof("Success to resize the tty!")
	return nil
}
