package docker

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/dockerversion"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/docker/docker/pkg/parsers/kernel"
	"github.com/docker/docker/pkg/parsers/operatingsystem"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/utils"
	"github.com/golang/glog"
)

func (cli *Docker) SendCmdInfo(args ...string) (*types.Info, error) {
	daemon := cli.daemon
	images := daemon.Graph().Map()
	var imgcount int
	if images == nil {
		imgcount = 0
	} else {
		imgcount = len(images)
	}
	kernelVersion := "<unknown>"
	if kv, err := kernel.GetKernelVersion(); err == nil {
		kernelVersion = kv.String()
	}

	operatingSystem := "<unknown>"
	if s, err := operatingsystem.GetOperatingSystem(); err == nil {
		operatingSystem = s
	}

	// Don't do containerized check on Windows
	if runtime.GOOS != "windows" {
		if inContainer, err := operatingsystem.IsContainerized(); err != nil {
			glog.Errorf("Could not determine if daemon is containerized: %v", err)
			operatingSystem += " (error determining if containerized)"
		} else if inContainer {
			operatingSystem += " (containerized)"
		}
	}

	meminfo, err := system.ReadMemInfo()
	if err != nil {
		glog.Errorf("Could not read system memory info: %v", err)
	}

	// if we still have the original dockerinit binary from before we copied it locally, let's return the path to that, since that's more intuitive (the copied path is trivial to derive by hand given VERSION)
	initPath := utils.DockerInitPath("")
	if initPath == "" {
		// if that fails, we'll just return the path from the daemon
		initPath = daemon.SystemInitPath()
	}

	v := &types.Info{
		ID:                 daemon.ID,
		Containers:         len(daemon.List()),
		Images:             imgcount,
		Driver:             daemon.GraphDriver().String(),
		DriverStatus:       daemon.GraphDriver().Status(),
		MemoryLimit:        daemon.SystemConfig().MemoryLimit,
		SwapLimit:          daemon.SystemConfig().SwapLimit,
		CPUCfsPeriod:       daemon.SystemConfig().CPUCfsPeriod,
		CPUCfsQuota:        daemon.SystemConfig().CPUCfsQuota,
		IPv4Forwarding:     !daemon.SystemConfig().IPv4ForwardingDisabled,
		Debug:              os.Getenv("DEBUG") != "",
		NFd:                fileutils.GetTotalUsedFds(),
		OomKillDisable:     daemon.SystemConfig().OomKillDisable,
		NGoroutines:        runtime.NumGoroutine(),
		SystemTime:         time.Now().Format(time.RFC3339Nano),
		NEventsListener:    daemon.EventsService.SubscribersCount(),
		KernelVersion:      kernelVersion,
		OperatingSystem:    operatingSystem,
		IndexServerAddress: registry.IndexServer,
		RegistryConfig:     daemon.RegistryService.Config,
		InitSha1:           dockerversion.InitSHA1,
		InitPath:           initPath,
		NCPU:               runtime.NumCPU(),
		MemTotal:           meminfo.MemTotal,
		DockerRootDir:      daemon.Config().Root,
		Labels:             daemon.Config().Labels,
		ExperimentalBuild:  utils.ExperimentalBuild(),
		ServerVersion:      dockerversion.Version,
		ClusterStore:       daemon.Config().ClusterStore,
		ClusterAdvertise:   daemon.Config().ClusterAdvertise,
		HTTPProxy:          getProxyEnv("http_proxy"),
		HTTPSProxy:         getProxyEnv("https_proxy"),
		NoProxy:            getProxyEnv("no_proxy"),
	}
	if hostname, err := os.Hostname(); err == nil {
		v.Name = hostname
	}

	return v, nil
}

// The uppercase and the lowercase are available for the proxy settings.
// See the Go specification for details on these variables. https://golang.org/pkg/net/http/
func getProxyEnv(key string) string {
	proxyValue := os.Getenv(strings.ToUpper(key))
	if proxyValue == "" {
		return os.Getenv(strings.ToLower(key))
	}
	return proxyValue
}
