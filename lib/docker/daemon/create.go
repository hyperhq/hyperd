package daemon

import (
	"fmt"
	"path/filepath"

	"github.com/hyperhq/hyper/lib/docker/graph"
	"github.com/hyperhq/hyper/lib/docker/image"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) ContainerCreate(name string, config *runconfig.Config, hostConfig *runconfig.HostConfig) (string, []string, error) {
	if config == nil {
		return "", nil, fmt.Errorf("Config cannot be empty in order to create a container")
	}

	warnings, err := daemon.verifyContainerSettings(hostConfig)
	if err != nil {
		return "", warnings, err
	}

	// The check for a valid workdir path is made on the server rather than in the
	// client. This is because we don't know the type of path (Linux or Windows)
	// to validate on the client.
	if config.WorkingDir != "" && !filepath.IsAbs(config.WorkingDir) {
		return "", warnings, fmt.Errorf("The working directory '%s' is invalid. It needs to be an absolute path.", config.WorkingDir)
	}

	container, buildWarnings, err := daemon.Create(config, hostConfig, name)
	if err != nil {
		if daemon.Graph().IsNotExist(err, config.Image) {
			_, tag := parsers.ParseRepositoryTag(config.Image)
			if tag == "" {
				tag = graph.DEFAULTTAG
			}
			return "", warnings, fmt.Errorf("No such image: %s (tag: %s)", config.Image, tag)
		}
		return "", warnings, err
	}

	warnings = append(warnings, buildWarnings...)

	return container.ID, warnings, nil
}

// Create creates a new container from the given configuration with a given name.
func (daemon *Daemon) Create(config *runconfig.Config, hostConfig *runconfig.HostConfig, name string) (*Container, []string, error) {
	var (
		container *Container
		warnings  []string
		img       *image.Image
		imgID     string
		err       error
	)

	if config.Image != "" {
		img, err = daemon.repositories.LookupImage(config.Image)
		if err != nil {
			glog.Errorf(err.Error())
			return nil, nil, err
		}
		if err = img.CheckDepth(); err != nil {
			return nil, nil, err
		}
		imgID = img.ID
	}

	if err := daemon.mergeAndVerifyConfig(config, img); err != nil {
		return nil, nil, err
	}
	if !config.NetworkDisabled && daemon.SystemConfig().IPv4ForwardingDisabled {
		warnings = append(warnings, "IPv4 forwarding is disabled.")
	}
	if hostConfig == nil {
		hostConfig = &runconfig.HostConfig{}
	}
	if container, err = daemon.newContainer(name, config, imgID); err != nil {
		return nil, nil, err
	}
	if err := daemon.Register(container); err != nil {
		return nil, nil, err
	}
	if err := daemon.createRootfs(container); err != nil {
		return nil, nil, err
	}
	if err := daemon.setHostConfig(container, hostConfig); err != nil {
		return nil, nil, err
	}
	if err := container.Mount(); err != nil {
		return nil, nil, err
	}
	defer container.Unmount()

	if err := container.ToDisk(); err != nil {
		return nil, nil, err
	}
	container.LogEvent("create")
	return container, warnings, nil
}
