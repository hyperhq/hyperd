package daemon

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdCommit(job *engine.Job) error {
	containerId := job.Args[0]
	repo := job.Args[1]
	author := job.Args[2]
	change := job.Args[3]
	message := job.Args[4]
	pause := job.Args[5]

	cli := daemon.DockerCli
	imgId, _, err := cli.SendContainerCommit(containerId, repo, author, change, message, pause)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	v := &engine.Env{}
	v.SetJson("ID", string(imgId))
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
