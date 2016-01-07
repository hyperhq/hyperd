package docker

import (
	"github.com/docker/docker/api/types"
	"github.com/golang/glog"
)

func (cli *Docker) GetContainerInfo(args ...string) (*types.ContainerJSON, error) {
	containerId := args[0]
	glog.V(1).Infof("ready to get the container(%s) info", containerId)
	containerJSON, err := cli.daemon.ContainerInspect(containerId, false)
	if err != nil {
		return nil, err
	}
	return containerJSON, nil
}

func (cli Docker) SendContainerRename(oldName, newName string) error {
	return cli.daemon.ContainerRename(oldName, newName)
}
