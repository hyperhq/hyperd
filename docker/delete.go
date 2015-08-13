package docker

import (
	"fmt"
	"strings"

	"github.com/hyperhq/hyper/lib/docker/daemon"
)

func (cli *Docker) SendCmdDelete(args ...string) ([]byte, int, error) {
	containerId := args[0]
	config := &daemon.ContainerRmConfig{
		ForceRemove:  true,
		RemoveVolume: true,
		RemoveLink:   false,
	}

	if err := cli.daemon.ContainerRm(containerId, config); err != nil {
		// Force a 404 for the empty string
		if strings.Contains(strings.ToLower(err.Error()), "prefix can't be empty") {
			return nil, 500, fmt.Errorf("no such id: \"\"")
		}
		return nil, 500, err
	}
	return nil, 200, nil
}
