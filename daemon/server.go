package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/distribution/digest"
	"github.com/docker/docker/reference"
	"github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/hyperd/lib/sysinfo"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
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
		if len(i.RepoTags) == 0 {
			slice := strings.Split(i.RepoDigests[0], "@")
			repoTag := slice[0] + ":" + "<none>"
			imagesList = append(imagesList, repoTag+":"+id[1]+":"+created+":"+size)
		}
	}

	v := &engine.Env{}
	v.SetList("imagesList", imagesList)

	return v, nil

}

func (daemon *Daemon) CmdAuthenticateToRegistry(config *types.AuthConfig) (string, error) {
	return daemon.Daemon.AuthenticateToRegistry(config)
}

func (daemon *Daemon) CmdAttach(stdin io.ReadCloser, stdout io.WriteCloser, container string) error {
	return daemon.Attach(stdin, stdout, container)
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

func (daemon *Daemon) CreateContainerInPod(podId string, spec *apitypes.UserContainer) (string, error) {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return "", fmt.Errorf("The pod(%s) can not be found", podId)
	}

	return p.ContainerCreate(spec)
}

func (daemon *Daemon) StartContainer(podId, containerId string) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("The pod(%s) can not be found", podId)
	}

	return p.ContainerStart(containerId)
}

func (daemon *Daemon) CmdCreateContainer(podId string, containerArgs []byte) (string, error) {
	var c apitypes.UserContainer

	err := json.Unmarshal(containerArgs, &c)
	if err != nil {
		glog.Errorf("Create container unmarshal failed: %v", err)
		return "", err
	}

	return daemon.CreateContainerInPod(podId, &c)
}

func (daemon *Daemon) CmdStartContainer(podId, containerId string) error {
	err := daemon.StartContainer(podId, containerId)
	if err != nil {
		glog.Errorf("fail to start container %s in pod %s: %v", containerId, podId, err)
		return err
	}
	return nil
}

func (daemon *Daemon) CmdKillContainer(name string, sig int64) (*engine.Env, error) {
	err := daemon.KillContainer(name, sig)
	if err != nil {
		glog.Errorf("fail to kill container %s with signal %d: %v", name, sig, err)
		return nil, err
	}
	v := &engine.Env{}
	return v, nil
}

func (daemon *Daemon) CmdStopContainer(name string) (*engine.Env, error) {
	err := daemon.StopContainer(name, 5)
	if err != nil {
		glog.Errorf("fail to stop container %s: %v", name, err)
		return nil, err
	}
	v := &engine.Env{}
	return v, nil
}

func (daemon *Daemon) CmdExitCode(containerId, execId string) (int, error) {
	return daemon.ExitCode(containerId, execId)
}

func (daemon *Daemon) CmdSystemInfo() (*apitypes.InfoResponse, error) {
	sys, err := daemon.Daemon.SystemInfo()
	if err != nil {
		return nil, err
	}

	var num = daemon.PodList.CountContainers()
	info := &apitypes.InfoResponse{
		ID:                 daemon.ID,
		Containers:         int32(num),
		Images:             int32(sys.Images),
		Driver:             sys.Driver,
		DockerRootDir:      sys.DockerRootDir,
		IndexServerAddress: sys.IndexServerAddress,
		ExecutionDriver:    daemon.Hypervisor,
	}

	for _, driverStatus := range sys.DriverStatus {
		info.Dstatus = append(info.Dstatus, &apitypes.DriverStatus{Name: driverStatus[0], Status: driverStatus[1]})
	}

	//Get system infomation
	meminfo, err := sysinfo.GetMemInfo()
	if err != nil {
		return nil, err
	}
	osinfo, err := sysinfo.GetOSInfo()
	if err != nil {
		return nil, err
	}

	info.MemTotal = int64(meminfo.MemTotal)
	info.Pods = daemon.GetPodNum()
	info.OperatingSystem = osinfo.PrettyName
	if hostname, err := os.Hostname(); err == nil {
		info.Name = hostname
	}

	return info, nil
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

func (daemon *Daemon) CmdStartPod(stdin io.ReadCloser, stdout io.WriteCloser, podId, vmId string, attach bool) (*engine.Env, error) {
	code, cause, err := daemon.StartPod(stdin, stdout, podId, attach)
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
func (daemon *Daemon) CmdCreatePod(podArgs string) (*engine.Env, error) {
	var podSpec apitypes.UserPod
	err := json.Unmarshal([]byte(podArgs), &podSpec)
	if err != nil {
		return nil, err
	}

	p, err := daemon.CreatePod("", &podSpec)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", p.Id())
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
	code, cause, err := daemon.RemovePod(podId)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)

	return v, nil
}

func (daemon *Daemon) CmdImageDelete(name string, force, prune bool) ([]*apitypes.ImageDelete, error) {
	list, err := daemon.Daemon.ImageDelete(name, force, prune)
	if err != nil {
		return nil, err
	}

	result := make([]*apitypes.ImageDelete, len(list))
	for index, img := range list {
		result[index] = &apitypes.ImageDelete{
			Untaged: img.Untagged,
			Deleted: img.Deleted,
		}
	}

	return result, nil
}

func (daemon *Daemon) CmdImagePull(image, tag string, authConfig *types.AuthConfig, metaHeaders map[string][]string, output io.Writer) error {
	// Special case: "pull -a" may send an image name with a
	// trailing :. This is ugly, but let's not break API
	// compatibility.
	image = strings.TrimSuffix(image, ":")

	var ref reference.Named
	ref, err := reference.ParseNamed(image)
	if err != nil {
		return err
	}

	if tag != "" {
		// The "tag" could actually be a digest.
		var dgst digest.Digest
		dgst, err = digest.ParseDigest(tag)
		if err == nil {
			ref, err = reference.WithDigest(ref, dgst)
		} else {
			ref, err = reference.WithTag(ref, tag)
		}
	}

	return daemon.Daemon.PullImage(ref, metaHeaders, authConfig, output)
}

func (daemon *Daemon) CmdImagePush(repo, tag string, authConfig *types.AuthConfig, metaHeaders map[string][]string, output io.Writer) error {
	// Special case: "pull -a" may send an image name with a
	// trailing :. This is ugly, but let's not break API
	// compatibility.
	repo = strings.TrimSuffix(repo, ":")

	var ref reference.Named
	ref, err := reference.ParseNamed(repo)
	if err != nil {
		return err
	}

	if tag != "" {
		ref, err = reference.WithTag(ref, tag)
		if err != nil {
			return err
		}
	}

	return daemon.Daemon.PushImage(ref, metaHeaders, authConfig, output)
}

func (daemon *Daemon) CmdStopPod(podId, stopVm string) (*engine.Env, error) {
	code, cause, err := daemon.StopPod(podId)
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

func (daemon *Daemon) CmdKillPod(podId, container string, sig int64) (*engine.Env, error) {
	err := daemon.KillPodContainers(podId, container, sig)
	if err != nil {
		glog.Errorf("fail to kill container %s in pod %s with signal %d: %v", container, podId, sig, err)
		return nil, err
	}
	v := &engine.Env{}
	return v, nil
}

func (daemon *Daemon) CmdTtyResize(containerId, execId string, h, w int) error {
	return daemon.TtyResize(containerId, execId, h, w)
}

func (daemon *Daemon) CmdAddService(podId, data string) (*engine.Env, error) {
	var srvs []*apitypes.UserService
	err := json.Unmarshal([]byte(data), &srvs)
	if err != nil {
		return nil, err
	}

	err = daemon.AddService(podId, srvs)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdUpdateService(podId, data string) (*engine.Env, error) {
	var srvs []*apitypes.UserService
	err := json.Unmarshal([]byte(data), &srvs)
	if err != nil {
		return nil, err
	}

	err = daemon.UpdateService(podId, srvs)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdDeleteService(podId, data string) (*engine.Env, error) {
	var srvs []*apitypes.UserService
	err := json.Unmarshal([]byte(data), &srvs)
	if err != nil {
		return nil, err
	}

	err = daemon.DeleteService(podId, srvs)
	if err != nil {
		return nil, err
	}

	v := &engine.Env{}
	v.Set("Result", "success")
	return v, nil
}

func (daemon *Daemon) CmdGetServices(podId string) ([]*apitypes.UserService, error) {
	return daemon.GetServices(podId)
}

func (daemon *Daemon) CmdPausePod(podId string) error {
	glog.V(1).Infof("Pause pod %s", podId)
	return daemon.PausePod(podId)
}

func (daemon *Daemon) CmdUnpausePod(podId string) error {
	glog.V(1).Infof("Unpause pod %s", podId)
	return daemon.UnpausePod(podId)
}
