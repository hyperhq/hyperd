package daemon

import (
	"fmt"

	"github.com/golang/glog"
)

func (daemon *Daemon) ContainerRename(oldname, newname string) error {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(oldname)
	if !ok {
		err := fmt.Errorf("caonnot find pod contains container %s to rename", oldname)
		glog.Error(err)
		return err
	}

	p.RenameContainer(id, newname)
	return nil
}
