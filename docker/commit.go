package docker

import (
	"encoding/json"

	"github.com/hyperhq/hyper/lib/docker/builder"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
)

func (cli Docker) SendContainerCommit(args ...string) ([]byte, int, error) {
	containerId := args[0]
	repo := args[1]
	author := args[2]
	change := args[3]
	message := args[4]
	pause := true
	if args[5] == "no" {
		pause = false
	}
	container, err := cli.daemon.Get(containerId)
	if err != nil {
		return nil, -1, err
	}
	var changes []string
	if err := json.Unmarshal([]byte(change), &changes); err != nil {
		return nil, -1, err
	}
	r, t := parsers.ParseRepositoryTag(repo)
	if t == "" {
		t = "latest"
	}
	containerCommitConfig := &daemon.ContainerCommitConfig{
		Pause:   pause,
		Repo:    r,
		Tag:     t,
		Author:  author,
		Comment: message,
		Changes: changes,
		Config:  container.Config,
	}
	imgID, err := builder.Commit(cli.daemon, containerId, containerCommitConfig)
	if err != nil {
		return nil, -1, err
	}

	return []byte(imgID), 0, nil
}
