package daemon

import (
	"github.com/golang/glog"
)

func (daemon *Daemon) ContainerRename(oldname, newname string) error {
	if err := daemon.Daemon.ContainerRename(oldname, newname); err != nil {
		return err
	}

	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodList.RUnlock()

	daemon.PodList.Find(func(p *Pod) bool {
		for _, c := range p.status.Containers {
			if c.Name == "/"+oldname {
				c.Name = "/" + newname
				return true
			}
		}
		return false
	})

	return nil
}
