package daemon

import (
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/docker/graph"
	"github.com/hyperhq/hyper/types"
)

func (daemon *Daemon) CmdPush(job *engine.Job) error {
	remote := job.Args[0]
	tempConfig := &types.ImagePushConfig{}
	if err := job.GetenvJson("ImagePushConfig", tempConfig); err != nil {
		return err
	}
	imagePushConfig := &graph.ImagePushConfig{
		MetaHeaders: tempConfig.MetaHeaders,
		AuthConfig:  tempConfig.AuthConfig,
		Tag:         tempConfig.Tag,
		OutStream:   job.Stdout,
	}
	err := daemon.DockerCli.SendCmdPush(remote, imagePushConfig)
	if err != nil {
		return err
	}
	return nil
}
