package docker

import (
	"github.com/docker/docker/api/types"
)

func (cli Docker) SendImageDelete(args ...string) ([]types.ImageDelete, error) {
	name := args[0]
	force := true
	noprune := true
	if args[1] == "yes" {
		force = true
	} else {
		force = false
	}
	if args[2] == "yes" {
		noprune = true
	} else {
		noprune = false
	}

	list, err := cli.daemon.ImageDelete(name, force, noprune)
	if err != nil {
		return nil, err
	}
	return list, nil
}
