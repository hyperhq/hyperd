package daemonbuilder

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/docker/docker/api"
	"github.com/docker/docker/builder"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/httputils"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/urlutil"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/runv/hypervisor"
	hypertypes "github.com/hyperhq/runv/hypervisor/types"
)

// Docker implements builder.Backend for the docker Daemon object.
type Docker struct {
	*daemon.Daemon
	OutOld      io.Writer
	AuthConfigs map[string]types.AuthConfig
	Archiver    *archive.Archiver
	hyper       *Hyper
}

type Hyper struct {
	CopyPods  map[string]string
	BasicPods map[string]string
	Ready     chan bool
	Status    chan *hypertypes.VmResponse
	Vm        *hypervisor.Vm
}

// ensure Docker implements builder.Backend
var _ builder.Backend = Docker{}

func (d *Docker) InitHyper() {
	d.hyper = &Hyper{
		make(map[string]string),
		make(map[string]string),
		make(chan bool, 1),
		nil, nil,
	}
}

// Pull tells Docker to pull image referenced by `name`.
func (d Docker) Pull(name string) (builder.Image, error) {
	ref, err := reference.ParseNamed(name)
	if err != nil {
		return nil, err
	}
	ref = reference.WithDefaultTag(ref)

	pullRegistryAuth := &types.AuthConfig{}
	if len(d.AuthConfigs) > 0 {
		// The request came with a full auth config file, we prefer to use that
		repoInfo, err := d.Daemon.RegistryService.ResolveRepository(ref)
		if err != nil {
			return nil, err
		}

		resolvedConfig := registry.ResolveAuthConfig(
			d.AuthConfigs,
			repoInfo.Index,
		)
		pullRegistryAuth = &resolvedConfig
	}

	if err := d.Daemon.PullImage(ref, nil, pullRegistryAuth, ioutils.NopWriteCloser(d.OutOld)); err != nil {
		return nil, err
	}
	return d.GetImage(name)
}

// GetImage looks up a Docker image referenced by `name`.
func (d Docker) GetImage(name string) (builder.Image, error) {
	img, err := d.Daemon.GetImage(name)
	if err != nil {
		return nil, err
	}
	return imgWrap{img}, nil
}

// ContainerUpdateCmd updates Path and Args for the container with ID cID.
func (d Docker) ContainerUpdateCmd(cID string, cmd []string) error {
	c, err := d.Daemon.GetContainer(cID)
	if err != nil {
		return err
	}
	c.Path = cmd[0]
	c.Args = cmd[1:]
	return nil
}

// GetCachedImage returns a reference to a cached image whose parent equals `parent`
// and runconfig equals `cfg`. A cache miss is expected to return an empty ID and a nil error.
func (d Docker) GetCachedImage(imgID string, cfg *container.Config) (string, error) {
	cache, err := d.Daemon.ImageGetCached(image.ID(imgID), cfg)
	if cache == nil || err != nil {
		return "", err
	}
	return cache.ID().String(), nil
}

// Following is specific to builder contexts

// DetectContextFromRemoteURL returns a context and in certain cases the name of the dockerfile to be used
// irrespective of user input.
// progressReader is only used if remoteURL is actually a URL (not empty, and not a Git endpoint).
func DetectContextFromRemoteURL(r io.ReadCloser, remoteURL string, createProgressReader func(in io.ReadCloser) io.ReadCloser) (context builder.ModifiableContext, dockerfileName string, err error) {
	switch {
	case remoteURL == "":
		context, err = builder.MakeTarSumContext(r)
	case urlutil.IsGitURL(remoteURL):
		context, err = builder.MakeGitContext(remoteURL)
	case urlutil.IsURL(remoteURL):
		context, err = builder.MakeRemoteContext(remoteURL, map[string]func(io.ReadCloser) (io.ReadCloser, error){
			httputils.MimeTypes.TextPlain: func(rc io.ReadCloser) (io.ReadCloser, error) {
				dockerfile, err := ioutil.ReadAll(rc)
				if err != nil {
					return nil, err
				}

				// dockerfileName is set to signal that the remote was interpreted as a single Dockerfile, in which case the caller
				// should use dockerfileName as the new name for the Dockerfile, irrespective of any other user input.
				dockerfileName = api.DefaultDockerfileName

				// TODO: return a context without tarsum
				return archive.Generate(dockerfileName, string(dockerfile))
			},
			// fallback handler (tar context)
			"": func(rc io.ReadCloser) (io.ReadCloser, error) {
				return createProgressReader(rc), nil
			},
		})
	default:
		err = fmt.Errorf("remoteURL (%s) could not be recognized as URL", remoteURL)
	}
	return
}
