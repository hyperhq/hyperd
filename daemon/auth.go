package daemon

import (
	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdAuth(job *engine.Job) error {
	cli := daemon.DockerCli
	status, err := cli.SendCmdAuth(job.Stdin)
	if err != nil {
		return err
	}
	v := &engine.Env{}
	v.Set("status", status)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}
	return nil
}
