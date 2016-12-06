package daemon

import (
	"fmt"
	"io"

	"github.com/golang/glog"
)

func (daemon *Daemon) Attach(stdin io.ReadCloser, stdout io.WriteCloser, container string) error {
	var (
		err error
	)

	p, id, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		err = fmt.Errorf("cannot find container %s", container)
		glog.Error(err)
		return err
	}

	rsp := make(chan error)
	err = p.Attach(id, stdin, stdout, rsp)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for attach!")
	}()

	err = <-rsp

	return err
}
