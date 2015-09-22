package docker

import (
	"github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (cli *Docker) GetContainerInfo(args ...string) (*types.ContainerJSONRaw, error) {
	containerId := args[0]
	glog.V(1).Infof("ready to get the container(%s) info", containerId)
	containerJSONRaw, err := cli.daemon.ContainerInspectRaw(containerId)
	if err != nil {
		return nil, err
	}
	return containerJSONRaw, nil
}

func (cli Docker) SendContainerRename(oldName, newName string) error {
	return cli.daemon.ContainerRename(oldName, newName)
}
