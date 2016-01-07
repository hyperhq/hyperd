package daemon

import (
	"github.com/docker/docker/graph"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/types"
)

func (daemon *Daemon) CmdPull(job *engine.Job) error {
	imgName := job.Args[0]
	tempConfig := &types.ImagePullConfig{}
	if err := job.GetenvJson("ImagePullConfig", tempConfig); err != nil {
		return err
	}
	config := &graph.ImagePullConfig{
		MetaHeaders: tempConfig.MetaHeaders,
		AuthConfig:  tempConfig.AuthConfig,
		OutStream:   job.Stdout,
	}
	cli := daemon.DockerCli
	_, _, err := cli.SendCmdPull(imgName, config)
	if err != nil {
		return err
	}
	return nil
}
