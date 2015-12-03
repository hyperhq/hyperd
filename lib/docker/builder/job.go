package builder

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/api"
	"github.com/hyperhq/hyper/lib/docker/builder/parser"
	"github.com/hyperhq/hyper/lib/docker/cliconfig"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/lib/docker/graph/tags"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/httputils"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
	"github.com/hyperhq/hyper/lib/docker/pkg/streamformatter"
	"github.com/hyperhq/hyper/lib/docker/pkg/urlutil"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/hyper/lib/docker/utils"
	rand "github.com/hyperhq/hyper/utils"
)

// whitelist of commands allowed for a commit/import
var validCommitCommands = map[string]bool{
	"entrypoint": true,
	"cmd":        true,
	"user":       true,
	"workdir":    true,
	"env":        true,
	"volume":     true,
	"expose":     true,
	"onbuild":    true,
}

type Config struct {
	DockerfileName string
	RemoteURL      string
	RepoName       string
	SuppressOutput bool
	NoCache        bool
	Remove         bool
	ForceRemove    bool
	Pull           bool
	Memory         int64
	MemorySwap     int64
	CpuShares      int64
	CpuPeriod      int64
	CpuQuota       int64
	CpuSetCpus     string
	CpuSetMems     string
	CgroupParent   string
	AuthConfig     *cliconfig.AuthConfig
	ConfigFile     *cliconfig.ConfigFile

	Stdout  io.Writer
	Context io.ReadCloser
	// When closed, the job has been cancelled.
	// Note: not all jobs implement cancellation.
	// See Job.Cancel() and Job.WaitCancelled()
	cancelled  chan struct{}
	cancelOnce sync.Once
}

// When called, causes the Job.WaitCancelled channel to unblock.
func (b *Config) Cancel() {
	b.cancelOnce.Do(func() {
		close(b.cancelled)
	})
}

// Returns a channel which is closed ("never blocks") when the job is cancelled.
func (b *Config) WaitCancelled() <-chan struct{} {
	return b.cancelled
}

func NewBuildConfig() *Config {
	return &Config{
		AuthConfig: &cliconfig.AuthConfig{},
		ConfigFile: &cliconfig.ConfigFile{},
		cancelled:  make(chan struct{}),
	}
}

func Build(d *daemon.Daemon, buildConfig *Config) error {
	var (
		repoName string
		tag      string
		context  io.ReadCloser
	)

	repoName, tag = parsers.ParseRepositoryTag(buildConfig.RepoName)
	if repoName != "" {
		if err := registry.ValidateRepositoryName(repoName); err != nil {
			glog.Error(err.Error())
			return err
		}
		if len(tag) > 0 {
			if err := tags.ValidateTagName(tag); err != nil {
				glog.Error(err.Error())
				return err
			}
		}
	}

	if buildConfig.RemoteURL == "" {
		context = ioutil.NopCloser(buildConfig.Context)
	} else if urlutil.IsGitURL(buildConfig.RemoteURL) {
		root, err := utils.GitClone(buildConfig.RemoteURL)
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		defer os.RemoveAll(root)

		c, err := archive.Tar(root, archive.Uncompressed)
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		context = c
	} else if urlutil.IsURL(buildConfig.RemoteURL) {
		f, err := httputils.Download(buildConfig.RemoteURL)
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		defer f.Body.Close()
		dockerFile, err := ioutil.ReadAll(f.Body)
		if err != nil {
			glog.Error(err.Error())
			return err
		}

		// When we're downloading just a Dockerfile put it in
		// the default name - don't allow the client to move/specify it
		buildConfig.DockerfileName = api.DefaultDockerfileName

		c, err := archive.Generate(buildConfig.DockerfileName, string(dockerFile))
		if err != nil {
			return err
		}
		context = c
	}
	defer context.Close()

	sf := streamformatter.NewJSONStreamFormatter()
	hyper, err := GetDaemon()
	if err != nil {
		glog.Error(err.Error())
		return err
	}
	vmId := "buildervm-" + rand.RandStr(10, "number")
	defer func() {
		glog.V(1).Infof("Kill VM(%s)...", vmId)
		hyper.KillVm(vmId)
	}()

	builder := &Builder{
		Daemon:      d,
		Name:        vmId,
		Hyperdaemon: hyper,
		OutStream: &streamformatter.StdoutFormater{
			Writer:          buildConfig.Stdout,
			StreamFormatter: sf,
		},
		ErrStream: &streamformatter.StderrFormater{
			Writer:          buildConfig.Stdout,
			StreamFormatter: sf,
		},
		Verbose:         !buildConfig.SuppressOutput,
		UtilizeCache:    !buildConfig.NoCache,
		Remove:          buildConfig.Remove,
		ForceRemove:     buildConfig.ForceRemove,
		Pull:            buildConfig.Pull,
		OutOld:          buildConfig.Stdout,
		StreamFormatter: sf,
		AuthConfig:      buildConfig.AuthConfig,
		ConfigFile:      buildConfig.ConfigFile,
		dockerfileName:  buildConfig.DockerfileName,
		cpuShares:       buildConfig.CpuShares,
		cpuPeriod:       buildConfig.CpuPeriod,
		cpuQuota:        buildConfig.CpuQuota,
		cpuSetCpus:      buildConfig.CpuSetCpus,
		cpuSetMems:      buildConfig.CpuSetMems,
		cgroupParent:    buildConfig.CgroupParent,
		memory:          buildConfig.Memory,
		memorySwap:      buildConfig.MemorySwap,
		cancelled:       buildConfig.WaitCancelled(),
	}

	id, err := builder.Run(context)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	if repoName != "" {
		return d.Repositories().Tag(repoName, tag, id, true)
	}
	return nil
}

func BuildFromConfig(d *daemon.Daemon, c *runconfig.Config, changes []string) (*runconfig.Config, error) {
	ast, err := parser.Parse(bytes.NewBufferString(strings.Join(changes, "\n")))
	if err != nil {
		return nil, err
	}

	// ensure that the commands are valid
	for _, n := range ast.Children {
		if !validCommitCommands[n.Value] {
			return nil, fmt.Errorf("%s is not a valid change command", n.Value)
		}
	}

	builder := &Builder{
		Daemon:        d,
		Config:        c,
		OutStream:     ioutil.Discard,
		ErrStream:     ioutil.Discard,
		disableCommit: true,
	}

	for i, n := range ast.Children {
		if err := builder.dispatch(i, n); err != nil {
			return nil, err
		}
	}

	return builder.Config, nil
}

func Commit(d *daemon.Daemon, name string, c *daemon.ContainerCommitConfig) (string, error) {
	container, err := d.Get(name)
	if err != nil {
		return "", err
	}

	if c.Config == nil {
		c.Config = &runconfig.Config{}
	}

	newConfig, err := BuildFromConfig(d, c.Config, c.Changes)
	if err != nil {
		return "", err
	}

	if err := runconfig.Merge(newConfig, container.Config); err != nil {
		return "", err
	}

	img, err := d.Commit(container, c.Repo, c.Tag, c.Comment, c.Author, c.Pause, newConfig)
	if err != nil {
		return "", err
	}

	return img.ID, nil
}
