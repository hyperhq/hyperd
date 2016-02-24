package daemon

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/sysinfo"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (daemon *Daemon) CmdImages(args, filter string, all bool) (*engine.Env, error) {
	var (
		imagesList = []string{}
	)

	images, err := daemon.Daemon.Images(args, filter, all)
	if err != nil {
		return nil, err
	}

	for _, i := range images {
		id := strings.Split(i.ID, ":")
		created := fmt.Sprintf("%d", i.Created)
		size := fmt.Sprintf("%d", i.VirtualSize)
		for _, r := range i.RepoTags {
			imagesList = append(imagesList, r+":"+id[1]+":"+created+":"+size)
		}
	}

	v := &engine.Env{}
	v.SetList("imagesList", imagesList)

	return v, nil

}

func (daemon *Daemon) CmdAuthenticateToRegistry(config *types.AuthConfig) (string, error) {
	return daemon.Daemon.AuthenticateToRegistry(config)
}

func (daemon *Daemon) CmdAttach(stdin io.ReadCloser, stdout io.WriteCloser, key, id, tag string) error {
	return daemon.Attach(stdin, stdout, key, id, tag)
}

func (daemon *Daemon) CmdCommitImage(name string, cfg *types.ContainerCommitConfig) (*engine.Env, error) {
	imgId, err := daemon.Daemon.Commit(name, cfg)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.SetJson("ID", imgId)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	return v, nil
}

func (daemon *Daemon) CmdCreateContainer(params types.ContainerCreateConfig) (*engine.Env, error) {
	res, err := daemon.Daemon.ContainerCreate(params)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.SetJson("ID", daemon.ID)
	v.Set("ContainerID", res.ID)
	glog.V(3).Infof("The ContainerID is %s", res.ID)

	return v, nil
}

func (daemon *Daemon) CmdExec(stdin io.ReadCloser, stdout io.WriteCloser, key, id, cmd, tag string) error {
	return daemon.Exec(stdin, stdout, key, id, cmd, tag)
}

func (daemon *Daemon) CmdExitCode(container, tag string) (int, error) {
	return daemon.ExitCode(container, tag)
}

func (daemon *Daemon) CmdSystemInfo() (*engine.Env, error) {
	sys, err := daemon.Daemon.SystemInfo()
	if err != nil {
		return nil, err
	}

	var num = daemon.PodList.CountContainers()
	v := &engine.Env{}
	v.Set("ID", daemon.ID)
	v.SetInt("Containers", int(num))
	v.SetInt("Images", sys.Images)
	v.Set("Driver", sys.Driver)
	v.SetJson("DriverStatus", sys.DriverStatus)
	v.Set("DockerRootDir", sys.DockerRootDir)
	v.Set("IndexServerAddress", sys.IndexServerAddress)
	v.Set("ExecutionDriver", daemon.Hypervisor)

	// Get system infomation
	meminfo, err := sysinfo.GetMemInfo()
	if err != nil {
		return nil, err
	}
	osinfo, err := sysinfo.GetOSInfo()
	if err != nil {
		return nil, err
	}
	v.SetInt64("MemTotal", int64(meminfo.MemTotal))
	v.SetInt64("Pods", daemon.GetPodNum())
	v.Set("Operating System", osinfo.PrettyName)
	if hostname, err := os.Hostname(); err == nil {
		v.SetJson("Name", hostname)
	}

	return v, nil
}

func (daemon *Daemon) CmdSystemVersion() *engine.Env {
	v := &engine.Env{}

	v.Set("ID", daemon.ID)
	v.Set("Version", fmt.Sprintf("\"%s\"", utils.VERSION))

	return v
}

func (daemon *Daemon) CmdGetPodInfo(podName string) (interface{}, error) {
	return daemon.GetPodInfo(podName)
}

func (daemon *Daemon) CmdGetPodStats(podId string) (interface{}, error) {
	return daemon.GetPodStats(podId)
}

func (daemon *Daemon) CmdGetContainerInfo(name string) (interface{}, error) {
	return daemon.GetContainerInfo(name)
}

func (daemon *Daemon) CmdList(item, podId, vmId string, auxiliary bool) (*engine.Env, error) {
	list, err := daemon.List(item, podId, vmId, auxiliary)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("item", item)

	for key, value := range list {
		v.SetList(key, value)
	}

	return v, nil
}

func (daemon *Daemon) CmdGetContainerLogs(container string, config *ContainerLogsConfig) (err error) {
	return daemon.GetContainerLogs(container, config)
}

func (daemon *Daemon) CmdSetPodLabels(podId string, override bool, labels map[string]string) (*engine.Env, error) {
	if err := daemon.SetPodLabels(podId, override, labels); err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", 0)
	v.Set("Cause", "")

	return v, nil
}

func (daemon *Daemon) CmdStartPod(stdin io.ReadCloser, stdout io.WriteCloser, podId, vmId, tag string) (*engine.Env, error) {
	code, cause, err := daemon.StartPod(stdin, stdout, podId, vmId, tag)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)

	return v, nil
}

//FIXME: there was a `config` argument passed by docker/builder, but we never processed it.
func (daemon *Daemon) CmdCreatePod(podArgs string, autoremove bool) (*engine.Env, error) {
	p, err := daemon.CreatePod("", podArgs, autoremove)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", p.id)
	v.SetInt("Code", 0)
	v.Set("Cause", "")

	return v, nil
}

func (daemon *Daemon) CmdContainerRename(oldname, newname string) (*engine.Env, error) {
	if err := daemon.ContainerRename(oldname, newname); err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", newname)
	v.SetInt("Code", 0)
	v.Set("Cause", "")

	return v, nil
}

func (daemon *Daemon) CmdCleanPod(podId string) (*engine.Env, error) {
	code, cause, err := daemon.CleanPod(podId)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)

	return v, nil
}

func (daemon *Daemon) CmdImageDelete(name string, force, prune bool) (*engine.Env, error) {
	imagesList := []string{}
	list, err := daemon.Daemon.ImageDelete(name, force, prune)
	if err != nil {
		return nil, err
	}
	// FIXME
	_ = list
	v := &engine.Env{}
	v.SetList("imagesList", imagesList)

	return v, nil
}

func (daemon *Daemon) CmdStopPod(podId, stopVm string) (*engine.Env, error) {
	code, cause, err := daemon.StopPod(podId, stopVm)
	if err != nil {
		return nil, err
	}

	// Prepare the VM status to client
	v := &engine.Env{}

	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)

	return v, nil
}

func (daemon *Daemon) CmdTtyResize(podId, tag string, h, w int) error {
	return daemon.TtyResize(podId, tag, h, w)
}

func (daemon *Daemon) CmdCreateVm(cpu, mem int, async bool) (*engine.Env, error) {
	vm, err := daemon.CreateVm(cpu, mem, async)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", vm.Id)
	v.SetInt("Code", 0)
	v.Set("Cause", "")

	return v, nil
}

func (daemon *Daemon) CmdKillVm(vmId string) (*engine.Env, error) {
	code, cause, err := daemon.KillVm(vmId)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)

	return v, nil
}

func (daemon *Daemon) CmdAddService(podId, data string) (*engine.Env, error) {
	err := daemon.AddService(podId, data)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdUpdateService(podId, data string) (*engine.Env, error) {
	err := daemon.UpdateService(podId, data)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdDeleteService(podId, data string) (*engine.Env, error) {
	err := daemon.DeleteService(podId, data)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdGetServices(podId string) ([]pod.UserService, error) {
	return daemon.GetServices(podId)
}

func (daemon *Daemon) CmdPausePod(podId string) error {
	glog.V(1).Infof("Pause pod %s", podId)
	return daemon.pausePod(podId)
}

func (daemon *Daemon) CmdUnpausePod(podId string) error {
	glog.V(1).Infof("Unpause pod %s", podId)
	return daemon.unpausePod(podId)
}
