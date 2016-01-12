package docker

import (
	"github.com/docker/docker/pkg/stringutils"
	"github.com/docker/docker/runconfig"
	"github.com/hyperhq/hyper/lib/docker/daemon"
)

func (cli Docker) SendCmdCreate(name, image string, cmds []string, userConfig interface{}) ([]byte, int, error) {
	config := &runconfig.Config{
		Image: image,
		Cmd:   stringutils.NewStrSlice(cmds...),
	}
	if userConfig != nil {
		config = userConfig.(*runconfig.Config)
	}
	hostConfig := &runconfig.HostConfig{}
	containerResp, err := cli.daemon.ContainerCreate(&daemon.ContainerCreateConfig{
		Name:            name,
		Config:          config,
		HostConfig:      hostConfig,
		AdjustCPUShares: false,
	})
	if err != nil {
		return nil, 500, err
	}
	return []byte(containerResp.ID), 200, nil
}
