package docker

import (
	"github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/lib/docker/graph"
)

func (cli Docker) SendCmdImages(all string) ([]*types.Image, error) {
	var (
		allBoolValue = false
	)
	if all == "yes" {
		allBoolValue = true
	}
	imagesConfig := graph.ImagesConfig{
		All: allBoolValue,
	}
	images, err := cli.daemon.Repositories().Images(&imagesConfig)
	if err != nil {
		return nil, err
	}

	return images, nil
}
