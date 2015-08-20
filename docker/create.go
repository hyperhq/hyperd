package docker

import (
	"github.com/hyperhq/hyper/lib/docker/runconfig"
)

func (cli Docker) SendCmdCreate(name, image string, cmds []string, userConfig interface{}) ([]byte, int, error) {
	config := runconfig.Config{
		Image: image,
		Cmd:   runconfig.NewCommand(cmds...),
	}
	if userConfig != nil {
		config = userConfig.(runconfig.Config)
	}
	hostConfig := &runconfig.HostConfig{}
	containerId, _, err := cli.daemon.ContainerCreate(name, &config, hostConfig)
	if err != nil {
		return nil, 500, err
	}
	return []byte(containerId), 200, nil
}
