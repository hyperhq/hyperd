package daemon

import (
	"github.com/hyperhq/hyper/lib/docker/image"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/runv/lib/glog"
)

type ContainerCommitConfig struct {
	Pause   bool
	Repo    string
	Tag     string
	Author  string
	Comment string
	Changes []string
	Config  *runconfig.Config
}

// Commit creates a new filesystem image from the current state of a container.
// The image can optionally be tagged into a repository
func (daemon *Daemon) Commit(container *Container, repository, tag, comment, author string, pause bool, config *runconfig.Config) (*image.Image, error) {
	rwTar, err := container.ExportRw()
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}
	defer func() {
		if rwTar != nil {
			rwTar.Close()
		}
	}()

	// Create a new image from the container's base layers + a new layer from container changes
	var (
		containerID, parentImageID string
		containerConfig            *runconfig.Config
	)

	if container != nil {
		containerID = container.ID
		parentImageID = container.ImageID
		containerConfig = container.Config
	}

	img, err := daemon.graph.Create(rwTar, containerID, parentImageID, comment, author, containerConfig, config)
	if err != nil {
		return nil, err
	}

	// Register the image if needed
	if repository != "" {
		if err := daemon.repositories.Tag(repository, tag, img.ID, true); err != nil {
			return img, err
		}
	}
	container.LogEvent("commit")
	return img, nil
}
