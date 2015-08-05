package docker

import (
	"github.com/hyperhq/hyper/lib/docker/graph"
)

func (cli Docker) SendCmdPush(remote string, imagePushConfig *graph.ImagePushConfig) error {
	err := cli.daemon.Repositories().Push(remote, imagePushConfig)
	if err != nil {
		return err
	}
	return nil
}
