package daemon

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
)

const DefaultPathEnv = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

type Container struct {
	CommonContainer

	AppArmorProfile string
	// ---- END OF TEMPORARY DECLARATION ----

}

func killProcessDirectly(container *Container) error {
	return nil
}

func (container *Container) setupContainerDns() error {
	return nil
}

func (container *Container) updateParentsHosts() error {
	return nil
}

func (container *Container) setupLinkedContainers() ([]string, error) {
	return nil, nil
}

func (container *Container) createDaemonEnvironment(linkedEnv []string) []string {
	return nil
}

func (container *Container) initializeNetworking() error {
	return nil
}

func (container *Container) setupWorkingDirectory() error {
	return nil
}

func populateCommand(c *Container, env []string) error {
	return nil
}

// GetSize, return real size, virtual size
func (container *Container) GetSize() (int64, int64) {
	// TODO Windows
	return 0, 0
}

func (container *Container) AllocateNetwork() error {

	// TODO Windows. This needs reworking with libnetwork. In the
	// proof-of-concept for //build conference, the Windows daemon
	// invoked eng.Job("allocate_interface) passing through
	// RequestedMac.

	return nil
}

func (container *Container) ExportRw() (archive.Archive, error) {
	if container.daemon == nil {
		return nil, fmt.Errorf("Can't load storage driver for unregistered container %s", container.ID)
	}
	glog.V(2).Infof("container address %p, daemon address %p", container, container.daemon)
	archive, err := container.daemon.Diff(container)
	//	archive, err := diff("/tmp/test1/", "")
	if err != nil {
		return nil, err
	}
	return ioutils.NewReadCloserWrapper(archive, func() error {
			err := archive.Close()
			return err
		}),
		nil
	return nil, nil
}

func (container *Container) ReleaseNetwork() {
	// TODO Windows. Rework with libnetwork
}

func (container *Container) RestoreNetwork() error {
	// TODO Windows. Rework with libnetwork
	return nil
}

func disableAllActiveLinks(container *Container) {
}

func (container *Container) DisableLink(name string) {
}

func (container *Container) UnmountVolumes(forceSyscall bool) error {
	return nil
}

func diff(id, parent string) (diff archive.Archive, err error) {

	// create pod

	// start or replace pod
	glog.Infof("Diff between %s and %s", id, parent)
	layerFs := "/tmp/test1"
	if parent == "" {
		archive, err := archive.Tar(layerFs, archive.Uncompressed)
		if err != nil {
			return nil, err
		}
		return ioutils.NewReadCloserWrapper(archive, func() error {
			err := archive.Close()
			return err
		}), nil
	}

	parentFs := "/tmp/test2"

	changes, err := archive.ChangesDirs(layerFs, parentFs)
	if err != nil {
		return nil, err
	}

	archive, err := archive.ExportChanges(layerFs, changes)
	if err != nil {
		return nil, err
	}

	return ioutils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		return err
	}), nil
}
