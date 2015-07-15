package daemon

import (
	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdPull(job *engine.Job) error {
	imgName := job.Args[0]
	cli := daemon.dockerCli
	_, _, err := cli.SendCmdPull(imgName)
	if err != nil {
		return err
	}
	return nil
}
