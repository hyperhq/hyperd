package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/graph/tags"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/pkg/progressreader"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/pkg/ulimit"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/runconfig"
	"github.com/hyperhq/hyper/lib/docker/builder"
	"github.com/hyperhq/hyper/lib/docker/builder/dockerfile"
	"github.com/hyperhq/hyper/lib/docker/daemon/daemonbuilder"
)

func (cli Docker) SendImageBuild(name string, size int, ctx io.ReadCloser) ([]byte, int, error) {
	var (
		authConfigs   = map[string]cliconfig.AuthConfig{}
		buildConfig   = &dockerfile.Config{}
		buildArgs     = map[string]string{}
		buildUlimits  = []*ulimit.Ulimit{}
		isolation     = "" // r.FormValue("isolation")
		ulimitsJSON   = "" // r.FormValue("ulimits")
		buildArgsJSON = "" //  r.FormValue("buildargs")
		remoteURL     = "" // r.FormValue("remote")
	)
	buildConfig.Remove = true
	buildConfig.Pull = true
	output := ioutils.NewWriteFlusher(os.Stdout)
	defer output.Close()
	sf := streamformatter.NewJSONStreamFormatter()
	errf := func(err error) error {
		// Do not write the error in the http output if it's still empty.
		// This prevents from writing a 200(OK) when there is an interal error.
		if !output.Flushed() {
			return err
		}
		return nil
	}
	buildConfig.DockerfileName = "" // r.FormValue("dockerfile")
	buildConfig.Verbose = true      // !httputils.BoolValue(r, "q")
	buildConfig.UseCache = true     // !httputils.BoolValue(r, "nocache")
	buildConfig.ForceRemove = true  // httputils.BoolValue(r, "forcerm")
	buildConfig.MemorySwap = 0      // httputils.Int64ValueOrZero(r, "memswap")
	buildConfig.Memory = 0          // httputils.Int64ValueOrZero(r, "memory")
	buildConfig.ShmSize = 0         // httputils.Int64ValueOrZero(r, "shmsize")
	buildConfig.CPUShares = 0       // httputils.Int64ValueOrZero(r, "cpushares")
	buildConfig.CPUPeriod = 0       // httputils.Int64ValueOrZero(r, "cpuperiod")
	buildConfig.CPUQuota = 0        // httputils.Int64ValueOrZero(r, "cpuquota")
	buildConfig.CPUSetCpus = ""     // r.FormValue("cpusetcpus")
	buildConfig.CPUSetMems = ""     // r.FormValue("cpusetmems")
	buildConfig.CgroupParent = ""   // r.FormValue("cgroupparent")

	if i := runconfig.IsolationLevel(isolation); i != "" {
		if !runconfig.IsolationLevel.IsValid(i) {
			return nil, -1, errf(fmt.Errorf("Unsupported isolation: %q", i))
		}
		buildConfig.Isolation = i
	}

	if ulimitsJSON != "" {
		if err := json.NewDecoder(strings.NewReader(ulimitsJSON)).Decode(&buildUlimits); err != nil {
			return nil, -1, errf(err)
		}
		buildConfig.Ulimits = buildUlimits
	}

	if buildArgsJSON != "" {
		if err := json.NewDecoder(strings.NewReader(buildArgsJSON)).Decode(&buildArgs); err != nil {
			return nil, -1, errf(err)
		}
		buildConfig.BuildArgs = buildArgs
	}

	uidMaps, gidMaps := cli.daemon.GetUIDGIDMaps()
	defaultArchiver := &archive.Archiver{
		Untar:   chrootarchive.Untar,
		UIDMaps: uidMaps,
		GIDMaps: gidMaps,
	}
	docker := &daemonbuilder.Docker{
		Daemon:      cli.daemon,
		OutOld:      output,
		AuthConfigs: authConfigs,
		Archiver:    defaultArchiver,
	}

	// Currently, only used if context is from a remote url.
	// The field `In` is set by DetectContextFromRemoteURL.
	// Look at code in DetectContextFromRemoteURL for more information.
	pReader := &progressreader.Config{
		// TODO: make progressreader streamformatter-agnostic
		Out:       output,
		Formatter: sf,
		Size:      int64(size),
		NewLines:  true,
		ID:        "Downloading context",
		Action:    remoteURL,
	}
	context, dockerfileName, err := daemonbuilder.DetectContextFromRemoteURL(ctx, remoteURL, pReader)
	if err != nil {
		return nil, -1, errf(err)
	}
	defer func() {
		if err := context.Close(); err != nil {
			logrus.Debugf("[BUILDER] failed to remove temporary context: %v", err)
		}
	}()
	buildConfig.DockerfileName = dockerfileName
	b, err := dockerfile.NewBuilder(cli.daemon, buildConfig, docker, builder.DockerIgnoreContext{ModifiableContext: context}, nil)
	if err != nil {
		return nil, -1, errf(err)
	}

	b.Stdout = &streamformatter.StdoutFormatter{Writer: output, StreamFormatter: sf}
	b.Stderr = &streamformatter.StderrFormatter{Writer: output, StreamFormatter: sf}

	imgID, err := b.Build()
	if err != nil {
		return nil, -1, errf(err)
	}

	repo, tag := parsers.ParseRepositoryTag(name)
	if err := registry.ValidateRepositoryName(repo); err != nil {
		return nil, -1, errf(err)
	}
	if len(tag) > 0 {
		if err := tags.ValidateTagName(tag); err != nil {
			return nil, -1, errf(err)
		}
	} else {
		tag = tags.DefaultTag
	}

	if err := cli.daemon.TagImage(repo, tag, string(imgID), true); err != nil {
		return nil, -1, errf(err)
	}
	return nil, 0, nil
}
