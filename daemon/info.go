package daemon

import (
	"fmt"
	"os"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/sysinfo"
	"github.com/hyperhq/hyper/utils"
)

func (daemon *Daemon) CmdInfo(job *engine.Job) error {
	cli := daemon.DockerCli
	sys, err := cli.SendCmdInfo("")
	if err != nil {
		return err
	}

	var num = 0
	for _, p := range daemon.PodList {
		num += len(p.Containers)
	}
	v := &engine.Env{}
	v.Set("ID", daemon.ID)
	v.SetInt("Containers", num)
	v.SetInt("Images", sys.Images)
	v.Set("Driver", sys.Driver)
	v.SetJson("DriverStatus", sys.DriverStatus)
	v.Set("DockerRootDir", sys.DockerRootDir)
	v.Set("IndexServerAddress", sys.IndexServerAddress)
	v.Set("ExecutionDriver", daemon.Hypervisor)

	// Get system infomation
	meminfo, err := sysinfo.GetMemInfo()
	if err != nil {
		return err
	}
	osinfo, err := sysinfo.GetOSInfo()
	if err != nil {
		return err
	}
	v.SetInt64("MemTotal", int64(meminfo.MemTotal))
	v.SetInt64("Pods", daemon.GetPodNum())
	v.Set("Operating System", osinfo.PrettyName)
	if hostname, err := os.Hostname(); err == nil {
		v.SetJson("Name", hostname)
	}
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) CmdVersion(job *engine.Job) error {
	v := &engine.Env{}
	v.Set("ID", daemon.ID)
	v.Set("Version", fmt.Sprintf("\"%s\"", utils.VERSION))
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}
	return nil
}
