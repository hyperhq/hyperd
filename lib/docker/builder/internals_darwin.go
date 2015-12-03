package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/lib/docker/pkg/stringid"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/hyper/utils"
)

func fixPermissions(source, destination string, uid, gid int, destExisted bool) error {
	// If the destination didn't already exist, or the destination isn't a
	// directory, then we should Lchown the destination. Otherwise, we shouldn't
	// Lchown the destination.
	destStat, err := os.Stat(destination)
	if err != nil {
		// This should *never* be reached, because the destination must've already
		// been created while untar-ing the context.
		return err
	}
	doChownDestination := !destExisted || !destStat.IsDir()

	// We Walk on the source rather than on the destination because we don't
	// want to change permissions on things we haven't created or modified.
	return filepath.Walk(source, func(fullpath string, info os.FileInfo, err error) error {
		// Do not alter the walk root iff. it existed before, as it doesn't fall under
		// the domain of "things we should chown".
		if !doChownDestination && (source == fullpath) {
			return nil
		}

		// Path is prefixed by source: substitute with destination instead.
		cleaned, err := filepath.Rel(source, fullpath)
		if err != nil {
			return err
		}

		fullpath = filepath.Join(destination, cleaned)
		return os.Lchown(fullpath, uid, gid)
	})
}

func (b *Builder) create() (*daemon.Container, error) {
	if b.image == "" && !b.noBaseImage {
		return nil, fmt.Errorf("Please provide a source image with `from` prior to run")
	}
	b.Config.Image = b.image
	config := *b.Config

	// Create the Pod

	podId := fmt.Sprintf("buildpod-%s", utils.RandStr(10, "alpha"))
	podString, err := MakeBasicPod(podId, b.image, b.Config.Cmd.Slice())
	if err != nil {
		return nil, err
	}
	err = b.Hyperdaemon.CreatePod(podId, podString, false)
	if err != nil {
		return nil, err
	}
	// Get the container
	var (
		containerId = ""
		c           *daemon.Container
	)
	ps, ok := b.Hyperdaemon.PodList.GetStatus(podId)
	if !ok {
		return nil, fmt.Errorf("Cannot find pod %s", podId)
	}
	for _, i := range ps.Containers {
		containerId = i.Id
	}
	c, err = b.Daemon.Get(containerId)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	b.TmpContainers[c.ID] = struct{}{}
	b.TmpPods[podId] = struct{}{}
	fmt.Fprintf(b.OutStream, " ---> Running in %s\n", stringid.TruncateID(c.ID))

	if config.Cmd.Len() > 0 {
		// override the entry point that may have been picked up from the base image
		s := config.Cmd.Slice()
		c.Path = s[0]
		c.Args = s[1:]
	} else {
		config.Cmd = runconfig.NewCommand()
	}

	return c, nil
}
