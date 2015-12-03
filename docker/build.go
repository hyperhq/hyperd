package docker

import (
	"io"
	"os"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/builder"
	"github.com/hyperhq/hyper/lib/docker/cliconfig"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
)

func (cli Docker) SendImageBuild(name string, context io.ReadCloser) ([]byte, int, error) {
	var (
		authConfig  = &cliconfig.AuthConfig{}
		configFile  = &cliconfig.ConfigFile{}
		buildConfig = builder.NewBuildConfig()
	)

	buildConfig.Remove = true
	buildConfig.Pull = true

	output := ioutils.NewWriteFlusher(os.Stdout)
	buildConfig.Stdout = output
	buildConfig.Context = context

	buildConfig.RemoteURL = ""         //r.FormValue("remote")
	buildConfig.DockerfileName = ""    // r.FormValue("dockerfile")
	buildConfig.RepoName = name        //r.FormValue("t")
	buildConfig.SuppressOutput = false //boolValue(r, "q")
	buildConfig.NoCache = false        //boolValue(r, "nocache")
	buildConfig.ForceRemove = true     //boolValue(r, "forcerm")
	buildConfig.AuthConfig = authConfig
	buildConfig.ConfigFile = configFile
	buildConfig.MemorySwap = 0    //int64ValueOrZero(r, "memswap")
	buildConfig.Memory = 0        //int64ValueOrZero(r, "memory")
	buildConfig.CpuShares = 0     //int64ValueOrZero(r, "cpushares")
	buildConfig.CpuPeriod = 0     //int64ValueOrZero(r, "cpuperiod")
	buildConfig.CpuQuota = 0      //int64ValueOrZero(r, "cpuquota")
	buildConfig.CpuSetCpus = ""   //r.FormValue("cpusetcpus")
	buildConfig.CpuSetMems = ""   //r.FormValue("cpusetmems")
	buildConfig.CgroupParent = "" //r.FormValue("cgroupparent")

	if err := builder.Build(cli.daemon, buildConfig); err != nil {
		glog.Error(err.Error())
		// Do not write the error in the http output if it's still empty.
		// This prevents from writing a 200(OK) when there is an interal error.
		return nil, -1, err
	}
	return nil, 0, nil
}
