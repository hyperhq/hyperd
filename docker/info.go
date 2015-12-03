package docker

import (
	"os"
	"runtime"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/lib/docker/dockerversion"
	"github.com/hyperhq/hyper/lib/docker/pkg/fileutils"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers/kernel"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers/operatingsystem"
	"github.com/hyperhq/hyper/lib/docker/pkg/system"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/hyper/lib/docker/utils"
)

func (cli *Docker) SendCmdInfo(args ...string) (*types.Info, error) {
	daemon := cli.daemon
	images, _ := daemon.Graph().Map()
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
		CpuCfsPeriod:       daemon.SystemConfig().CpuCfsPeriod,
		CpuCfsQuota:        daemon.SystemConfig().CpuCfsQuota,
		IPv4Forwarding:     !daemon.SystemConfig().IPv4ForwardingDisabled,
		Debug:              os.Getenv("DEBUG") != "",
		NFd:                fileutils.GetTotalUsedFds(),
		OomKillDisable:     daemon.SystemConfig().OomKillDisable,
		NGoroutines:        runtime.NumGoroutine(),
		SystemTime:         time.Now().Format(time.RFC3339Nano),
		NEventsListener:    daemon.EventsService.SubscribersCount(),
		KernelVersion:      kernelVersion,
		OperatingSystem:    operatingSystem,
		IndexServerAddress: registry.IndexServerAddress(),
		RegistryConfig:     daemon.RegistryService.Config,
		InitSha1:           dockerversion.INITSHA1,
		InitPath:           initPath,
		NCPU:               runtime.NumCPU(),
		MemTotal:           meminfo.MemTotal,
		DockerRootDir:      daemon.Config().Root,
		Labels:             daemon.Config().Labels,
		ExperimentalBuild:  utils.ExperimentalBuild(),
	}

	if httpProxy := os.Getenv("http_proxy"); httpProxy != "" {
		v.HttpProxy = httpProxy
	}
	if httpsProxy := os.Getenv("https_proxy"); httpsProxy != "" {
		v.HttpsProxy = httpsProxy
	}
	if noProxy := os.Getenv("no_proxy"); noProxy != "" {
		v.NoProxy = noProxy
	}
	if hostname, err := os.Hostname(); err == nil {
		v.Name = hostname
	}

	return v, nil
}
