package daemon

import (
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdBuild(job *engine.Job) error {
	imgName := job.Args[0]
	content := job.Stdin

	cli := daemon.DockerCli
	_, _, err := cli.SendImageBuild(imgName, content)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	v := &engine.Env{}
	v.SetJson("ID", daemon.ID)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
