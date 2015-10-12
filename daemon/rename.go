package daemon

import (
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdRename(job *engine.Job) error {
	oldname := job.Args[0]
	newname := job.Args[1]
	cli := daemon.DockerCli
	err := cli.SendContainerRename(oldname, newname)
	if err != nil {
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

	v := &engine.Env{}
	v.Set("ID", newname)
	v.SetInt("Code", 0)
	v.Set("Cause", "")

	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
