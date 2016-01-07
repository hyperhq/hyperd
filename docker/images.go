package docker

import "github.com/docker/docker/api/types"

func (cli Docker) SendCmdImages(all string) ([]*types.Image, error) {
	var (
		allBoolValue = false
	)
	if all == "yes" {
		allBoolValue = true
	}
	images, err := cli.daemon.ListImages("", "", allBoolValue)
	if err != nil {
		return nil, err
	}

	return images, nil
}
