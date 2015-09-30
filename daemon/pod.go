package daemon

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hyperhq/hyper/engine"
	dockertypes "github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/servicediscovery"
	"github.com/hyperhq/hyper/storage"
	"github.com/hyperhq/hyper/storage/aufs"
	dm "github.com/hyperhq/hyper/storage/devicemapper"
	"github.com/hyperhq/hyper/storage/overlay"
	"github.com/hyperhq/hyper/storage/vbox"
	"github.com/hyperhq/hyper/utils"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdPodCreate(job *engine.Job) error {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}
	podArgs := job.Args[0]

	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()
	err := daemon.CreatePod(podId, podArgs, nil, false)
	if err != nil {
		return err
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdPodStart(job *engine.Job) error {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	var (
		tag         string              = ""
		ttys        []*hypervisor.TtyIO = []*hypervisor.TtyIO{}
		ttyCallback chan *types.VmResponse
	)

	podId := job.Args[0]
	vmId := job.Args[1]
	if len(job.Args) > 2 {
		tag = job.Args[2]
	}
	if tag != "" {
		glog.V(1).Info("Pod Run with client terminal tag: ", tag)
		ttyCallback = make(chan *types.VmResponse, 1)
		ttys = append(ttys, &hypervisor.TtyIO{
			Stdin:     job.Stdin,
			Stdout:    job.Stdout,
			ClientTag: tag,
			Callback:  ttyCallback,
		})
	}

	glog.Infof("pod:%s, vm:%s", podId, vmId)
	// Do the status check for the given pod
	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	if _, ok := daemon.PodList[podId]; !ok {
		glog.V(2).Infof("unlock PodList")
		daemon.PodsMutex.Unlock()
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode() && vmId == ""

	code, cause, err := daemon.StartPod(podId, "", vmId, nil, lazy, false, types.VM_KEEP_NONE, ttys)
	if err != nil {
		glog.Error(err.Error())
		glog.V(2).Infof("unlock PodList")
		daemon.PodsMutex.Unlock()
		return err
	}

	if len(ttys) > 0 {
		glog.V(2).Infof("unlock PodList")
		daemon.PodsMutex.Unlock()
		<-ttyCallback
		return nil
	}
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdPodRun(job *engine.Job) error {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}
	var (
		autoremove  bool                = false
		tag         string              = ""
		ttys        []*hypervisor.TtyIO = []*hypervisor.TtyIO{}
		ttyCallback chan *types.VmResponse
	)
	podArgs := job.Args[0]
	if job.Args[1] == "yes" {
		autoremove = true
	}
	if len(job.Args) > 2 {
		tag = job.Args[2]
	}

	if tag != "" {
		glog.V(1).Info("Pod Run with client terminal tag: ", tag)
		ttyCallback = make(chan *types.VmResponse, 1)
		ttys = append(ttys, &hypervisor.TtyIO{
			Stdin:     job.Stdin,
			Stdout:    job.Stdout,
			ClientTag: tag,
			Callback:  ttyCallback,
		})
	}

	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))

	glog.Info(podArgs)

	var lazy bool = hypervisor.HDriver.SupportLazyMode()

	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()
	code, cause, err := daemon.StartPod(podId, podArgs, "", nil, lazy, autoremove, types.VM_KEEP_NONE, ttys)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	if len(ttys) > 0 {
		<-ttyCallback
		return nil
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CreatePod(podId, podArgs string, config interface{}, autoremove bool) (err error) {
	glog.V(1).Infof("podArgs: %s", podArgs)
	var (
		userPod      *pod.UserPod
		containerIds []string
		cId          []byte
	)
	userPod, err = daemon.ProcessPodBytes([]byte(podArgs), podId)
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return err
	}

	if err = userPod.Validate(); err != nil {
		return err
	}

	mypod := hypervisor.NewPod(podId, userPod)
	mypod.Handler.Handle = hyperHandlePodEvent
	mypod.Handler.Data = daemon
	mypod.Autoremove = autoremove

	defer func() {
		if err != nil {
			if containerIds == nil {
				daemon.DeletePodFromDB(podId)
				if mypod != nil {
					for _, c := range mypod.Containers {
						glog.V(1).Infof("Ready to rm container: %s", c.Id)
						if _, _, err = daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
							glog.Warningf("Error to rm container: %s", err.Error())
						}
					}
				}
				daemon.RemovePod(podId)
				daemon.DeletePodContainerFromDB(podId)
			}
		}
	}()
	// store the UserPod into the db
	if err = daemon.WritePodToDB(podId, []byte(podArgs)); err != nil {
		glog.V(1).Info("Found an error while saveing the POD file")
		return err
	}
	containerIds, err = daemon.GetPodContainersByName(podId)
	if err != nil {
		glog.V(1).Info(err.Error())
	}

	if containerIds != nil {
		for _, id := range containerIds {
			var (
				name  string
				image string
			)
			if jsonResponse, err := daemon.DockerCli.GetContainerInfo(id); err == nil {
				name = jsonResponse.Name
				image = jsonResponse.Config.Image
			}
			mypod.AddContainer(id, name, image, []string{}, types.S_POD_CREATED)
		}
	} else {
		// Process the 'Containers' section
		glog.V(1).Info("Process the Containers section in POD SPEC")
		for _, c := range userPod.Containers {
			imgName := c.Image
			cId, _, err = daemon.DockerCli.SendCmdCreate(c.Name, imgName, []string{}, nil)
			if err != nil {
				glog.Error(err.Error())
				return err
			}
			var (
				name  string
				image string
			)
			if jsonResponse, err := daemon.DockerCli.GetContainerInfo(string(cId)); err == nil {
				name = jsonResponse.Name
				image = jsonResponse.Config.Image
			}

			mypod.AddContainer(string(cId), name, image, []string{}, types.S_POD_CREATED)
		}
	}

	daemon.AddPod(mypod)

	if err = daemon.WritePodAndContainers(podId); err != nil {
		glog.V(1).Info("Found an error while saveing the Containers info")
		return err
	}

	return nil
}

func (daemon *Daemon) PrepareContainer(mypod *hypervisor.Pod, userPod *pod.UserPod,
	vmId string) ([]*hypervisor.ContainerInfo, error) {
	var (
		fstype            string
		poolName          string
		volPoolName       string
		devPrefix         string
		rootPath          string
		devFullName       string
		rootfs            string
		err               error
		sharedDir         = path.Join(hypervisor.BaseDir, vmId, hypervisor.ShareDirTag)
		containerInfoList = []*hypervisor.ContainerInfo{}
		storageDriver     = daemon.Storage.StorageType
		cli               = daemon.DockerCli
	)

	if storageDriver == "devicemapper" {
		poolName = daemon.Storage.PoolName
		fstype = daemon.Storage.Fstype
		volPoolName = "hyper-volume-pool"
		devPrefix = poolName[:strings.Index(poolName, "-pool")]
		rootPath = path.Join(utils.HYPER_ROOT, "devicemapper")
		rootfs = "/rootfs"
	} else if storageDriver == "aufs" {
		rootPath = daemon.Storage.RootPath
		fstype = daemon.Storage.Fstype
		rootfs = ""
	} else if storageDriver == "overlay" {
		rootPath = daemon.Storage.RootPath
		fstype = daemon.Storage.Fstype
		rootfs = ""
	} else if storageDriver == "vbox" {
		fstype = daemon.Storage.Fstype
		rootPath = daemon.Storage.RootPath
		rootfs = "/rootfs"
		_ = devPrefix
		_ = volPoolName
		_ = poolName
	}

	// Process the 'Files' section
	files := make(map[string](pod.UserFile))
	for _, v := range userPod.Files {
		files[v.Name] = v
	}

	for i, c := range mypod.Containers {
		var jsonResponse *dockertypes.ContainerJSONRaw
		if jsonResponse, err = cli.GetContainerInfo(c.Id); err != nil {
			glog.Error("got error when get container Info ", err.Error())
			return nil, err
		}
		if c.Name == "" {
			c.Name = jsonResponse.Name
		}
		if c.Image == "" {
			c.Image = jsonResponse.Config.Image
		}

		if storageDriver == "devicemapper" {
			if err := dm.CreateNewDevice(c.Id, devPrefix, rootPath); err != nil {
				return nil, err
			}
			devFullName, err = dm.MountContainerToSharedDir(c.Id, sharedDir, devPrefix)
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return nil, err
			}
			fstype, err = dm.ProbeFsType(devFullName)
			if err != nil {
				fstype = "ext4"
			}
		} else if storageDriver == "aufs" {
			devFullName, err = aufs.MountContainerToSharedDir(c.Id, rootPath, sharedDir, "")
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return nil, err
			}
			devFullName = "/" + c.Id + "/rootfs"
		} else if storageDriver == "overlay" {
			devFullName, err = overlay.MountContainerToSharedDir(c.Id, rootPath, sharedDir, "")
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return nil, err
			}
			devFullName = "/" + c.Id + "/rootfs"
		} else if storageDriver == "vbox" {
			devFullName, err = vbox.MountContainerToSharedDir(c.Id, rootPath, "")
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return nil, err
			}
			fstype = "ext4"
		}

		for _, f := range userPod.Containers[i].Files {
			targetPath := f.Path
			if strings.HasSuffix(targetPath, "/") {
				targetPath = targetPath + f.Filename
			}
			file, ok := files[f.Filename]
			if !ok {
				continue
			}

			var src io.Reader

			if file.Uri != "" {
				urisrc, err := utils.UriReader(file.Uri)
				if err != nil {
					return nil, err
				}
				defer urisrc.Close()
				src = urisrc
			} else {
				src = strings.NewReader(file.Contents)
			}

			switch file.Encoding {
			case "base64":
				src = base64.NewDecoder(base64.StdEncoding, src)
			default:
			}

			if storageDriver == "devicemapper" {
				err := dm.InjectFile(src, c.Id, devPrefix, targetPath, rootPath,
					utils.PermInt(f.Perm), utils.UidInt(f.User), utils.UidInt(f.Group))
				if err != nil {
					glog.Error("got error when inject files ", err.Error())
					return nil, err
				}
			} else if storageDriver == "aufs" || storageDriver == "overlay" {
				err := storage.FsInjectFile(src, c.Id, targetPath, sharedDir,
					utils.PermInt(f.Perm), utils.UidInt(f.User), utils.UidInt(f.Group))
				if err != nil {
					glog.Error("got error when inject files ", err.Error())
					return nil, err
				}
			}
		}

		env := make(map[string]string)
		for _, v := range jsonResponse.Config.Env {
			env[v[:strings.Index(v, "=")]] = v[strings.Index(v, "=")+1:]
		}
		for _, e := range userPod.Containers[i].Envs {
			env[e.Env] = e.Value
		}
		glog.V(1).Infof("Parsing envs for container %d: %d Evs", i, len(env))
		glog.V(1).Infof("The fs type is %s", fstype)
		glog.V(1).Infof("WorkingDir is %s", string(jsonResponse.Config.WorkingDir))
		glog.V(1).Infof("Image is %s", string(devFullName))
		containerInfo := &hypervisor.ContainerInfo{
			Id:         c.Id,
			Rootfs:     rootfs,
			Image:      devFullName,
			Fstype:     fstype,
			Workdir:    jsonResponse.Config.WorkingDir,
			Entrypoint: jsonResponse.Config.Entrypoint.Slice(),
			Cmd:        jsonResponse.Config.Cmd.Slice(),
			Envs:       env,
		}
		glog.V(1).Infof("Container Info is \n%v", containerInfo)
		containerInfoList = append(containerInfoList, containerInfo)
		glog.V(1).Infof("container %d created %s, workdir %s, env: %v", i, c.Id, jsonResponse.Config.WorkingDir, env)
	}

	return containerInfoList, nil
}

func (daemon *Daemon) PrepareServices(userPod *pod.UserPod, podId string) error {
	err := servicediscovery.PrepareServices(userPod, podId)
	if err != nil {
		glog.Errorf("PrepareServices failed %s", err.Error())
	}
	return err
}

func (daemon *Daemon) PrepareVolume(mypod *hypervisor.Pod, userPod *pod.UserPod,
	vmId string) ([]*hypervisor.VolumeInfo, error) {
	var (
		fstype         string
		volPoolName    string
		err            error
		sharedDir      = path.Join(hypervisor.BaseDir, vmId, hypervisor.ShareDirTag)
		volumeInfoList = []*hypervisor.VolumeInfo{}
	)

	// Process the 'Volumes' section
	for _, v := range userPod.Volumes {
		if v.Source == "" {
			if daemon.Storage.StorageType == "devicemapper" {
				volName := fmt.Sprintf("%s-%s-%s", volPoolName, mypod.Id, v.Name)
				dev_id, _ := daemon.GetVolumeId(mypod.Id, volName)
				glog.Warningf("DeviceID is %d", dev_id)
				if dev_id < 1 {
					dev_id, _ = daemon.GetMaxDeviceId()
					err := daemon.CreateVolume(mypod.Id, volName, fmt.Sprintf("%d", dev_id+1), false)
					if err != nil {
						return nil, err
					}
				} else {
					err := daemon.CreateVolume(mypod.Id, volName, fmt.Sprintf("%d", dev_id), true)
					if err != nil {
						return nil, err
					}
				}

				fstype, err = dm.ProbeFsType("/dev/mapper/" + volName)
				if err != nil {
					fstype = "ext4"
				}
				myVol := &hypervisor.VolumeInfo{
					Name:     v.Name,
					Filepath: path.Join("/dev/mapper/", volName),
					Fstype:   fstype,
					Format:   "raw",
				}
				volumeInfoList = append(volumeInfoList, myVol)
				glog.V(1).Infof("volume %s created with dm as %s", v.Name, volName)
				continue
			} else {
				// Make sure the v.Name is given
				v.Source = path.Join("/var/tmp/hyper/", v.Name)
				if _, err := os.Stat(v.Source); err != nil && os.IsNotExist(err) {
					if err := os.MkdirAll(v.Source, os.FileMode(0777)); err != nil {
						return nil, err
					}
				}
				v.Driver = "vfs"
			}
		}

		if v.Driver != "vfs" {
			glog.V(1).Infof("bypass %s volume %s", v.Driver, v.Name)
			continue
		}

		// Process the situation if the source is not NULL, we need to bind that dir to sharedDir
		var flags uintptr = utils.MS_BIND

		mountSharedDir := pod.RandStr(10, "alpha")
		targetDir := path.Join(sharedDir, mountSharedDir)
		glog.V(1).Infof("trying to bind dir %s to %s", v.Source, targetDir)

		stat, err := os.Stat(v.Source)
		if err != nil {
			glog.Error("Cannot stat volume Source ", err.Error())
			return nil, err
		}

		if runtime.GOOS == "linux" {
			base := filepath.Base(targetDir)
			if err := os.MkdirAll(base, 0755); err != nil && !os.IsExist(err) {
				glog.Errorf("error to create dir %s for volume %s", base, v.Name)
				return nil, err
			}

			if stat.IsDir() {
				if err := os.MkdirAll(targetDir, 0755); err != nil && !os.IsExist(err) {
					glog.Errorf("error to create dir %s for volume %s", targetDir, v.Name)
					return nil, err
				}
			} else if f, err := os.Create(targetDir); err != nil && !os.IsExist(err) {
				glog.Errorf("error to create file %s for volume %s", targetDir, v.Name)
				return nil, err
			} else if err == nil {
				f.Close()
			}
		}

		if err := utils.Mount(v.Source, targetDir, "none", flags, "--bind"); err != nil {
			glog.Errorf("bind dir %s failed: %s", v.Source, err.Error())
			return nil, err
		}
		myVol := &hypervisor.VolumeInfo{
			Name:     v.Name,
			Filepath: mountSharedDir,
			Fstype:   "dir",
			Format:   "",
		}
		glog.V(1).Infof("dir %s is bound to %s", v.Source, targetDir)
		volumeInfoList = append(volumeInfoList, myVol)
	}

	return volumeInfoList, nil
}

func (daemon *Daemon) PreparePod(mypod *hypervisor.Pod, userPod *pod.UserPod,
	vmId string) ([]*hypervisor.ContainerInfo, []*hypervisor.VolumeInfo, error) {

	daemon.PrepareServices(userPod, mypod.Id)

	containerInfoList, err := daemon.PrepareContainer(mypod, userPod, vmId)
	if err != nil {
		return nil, nil, err
	}

	volumeInfoList, err := daemon.PrepareVolume(mypod, userPod, vmId)
	if err != nil {
		return nil, nil, err
	}

	return containerInfoList, volumeInfoList, nil
}

func (daemon *Daemon) StartPod(podId, podArgs, vmId string, config interface{}, lazy, autoremove bool, keep int, streams []*hypervisor.TtyIO) (int, string, error) {
	glog.V(1).Infof("podArgs: %s", podArgs)
	var (
		podData []byte
		err     error
		mypod   *hypervisor.Pod
		vm      *hypervisor.Vm = nil
	)

	mypod, podData, err = daemon.GetPod(podId, podArgs, autoremove)
	if err != nil {
		return -1, "", err
	}

	userPod, err := daemon.ProcessPodBytes(podData, podId)
	if err != nil {
		return -1, "", err
	}

	if !userPod.Tty && streams != nil && len(streams) > 0 {
		cause := "Spec does not support TTY, but IO streams are provided"
		return -1, cause, errors.New(cause)
	}

	vm, err = daemon.GetVM(vmId, &userPod.Resource, lazy, keep)
	if err != nil {
		return -1, "", err
	}

	defer func() {
		if vm != nil && err != nil && vmId == "" {
			daemon.KillVm(vm.Id)
		}
	}()

	containerInfoList, volumeInfoList, err := daemon.PreparePod(mypod, userPod, vm.Id)
	if err != nil {
		return -1, "", err
	}

	for idx, str := range streams {
		if idx >= len(userPod.Containers) {
			break
		}

		err = vm.Attach(str.Stdin, str.Stdout, str.ClientTag, containerInfoList[idx].Id, str.Callback, nil)
		if err != nil {
			glog.Errorf("Failed to attach client %s before start pod", str.ClientTag)
			return -1, "", err
		}
		glog.V(1).Infof("Attach client %s before start pod", str.ClientTag)
	}

	vmResponse := vm.StartPod(mypod, userPod, containerInfoList, volumeInfoList)
	if streams != nil && len(streams) > 0 && vmResponse.Code == types.E_OK {
		return 0, "", nil
	}
	if vmResponse.Data == nil {
		err = fmt.Errorf("VM response data is nil")
		return vmResponse.Code, vmResponse.Cause, err
	}
	data := vmResponse.Data.([]byte)
	err = daemon.UpdateVmData(vm.Id, data)
	if err != nil {
		glog.Error(err.Error())
		return -1, "", err
	}
	// add or update the Vm info for POD
	if err := daemon.UpdateVmByPod(podId, vm.Id); err != nil {
		glog.Error(err.Error())
		return -1, "", err
	}

	// XXX we should not close vmStatus chan, it will be closed in shutdown process
	return vmResponse.Code, vmResponse.Cause, nil
}

// The caller must make sure that the restart policy and the status is right to restart
func (daemon *Daemon) RestartPod(mypod *hypervisor.Pod) error {
	// Remove the pod
	// The pod is stopped, the vm is gone
	for _, c := range mypod.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
			glog.V(1).Infof("Error to rm container: %s", err.Error())
		}
	}
	daemon.RemovePod(mypod.Id)
	daemon.DeletePodContainerFromDB(mypod.Id)
	daemon.DeleteVolumeId(mypod.Id)

	podData, err := daemon.GetPodByName(mypod.Id)
	if err != nil {
		return err
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode()

	// Start the pod
	_, _, err = daemon.StartPod(mypod.Id, string(podData), "", nil, lazy, false, types.VM_KEEP_NONE, []*hypervisor.TtyIO{})
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	if err := daemon.WritePodAndContainers(mypod.Id); err != nil {
		glog.Error("Found an error while saving the Containers info")
		return err
	}

	return nil
}

func hyperHandlePodEvent(vmResponse *types.VmResponse, data interface{},
	mypod *hypervisor.Pod, vm *hypervisor.Vm) bool {
	daemon := data.(*Daemon)

	if vmResponse.Code == types.E_POD_FINISHED {
		if vm.Keep != types.VM_KEEP_NONE {
			mypod.Vm = ""
			vm.Status = types.S_VM_IDLE
			return false
		}
		mypod.SetPodContainerStatus(vmResponse.Data.([]uint32))
		mypod.Vm = ""
		vm.Status = types.S_VM_IDLE
		if mypod.Autoremove == true {
			daemon.CleanPod(mypod.Id)
			return false
		}
	} else if vmResponse.Code == types.E_VM_SHUTDOWN {
		if mypod.Status == types.S_POD_RUNNING {
			mypod.Status = types.S_POD_SUCCEEDED
			mypod.SetContainerStatus(types.S_POD_SUCCEEDED)
		}
		mypod.Vm = ""
		daemon.RemoveVm(vm.Id)
		if mypod.Type == "kubernetes" {
			switch mypod.Status {
			case types.S_POD_SUCCEEDED:
				if mypod.RestartPolicy == "always" {
					daemon.RestartPod(mypod)
					break
				}
				daemon.DeletePodFromDB(mypod.Id)
				for _, c := range mypod.Containers {
					glog.V(1).Infof("Ready to rm container: %s", c.Id)
					if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
						glog.V(1).Infof("Error to rm container: %s", err.Error())
					}
				}
				daemon.DeletePodContainerFromDB(mypod.Id)
				daemon.DeleteVolumeId(mypod.Id)
				break
			case types.S_POD_FAILED:
				if mypod.RestartPolicy != "never" {
					daemon.RestartPod(mypod)
					break
				}
				daemon.DeletePodFromDB(mypod.Id)
				for _, c := range mypod.Containers {
					glog.V(1).Infof("Ready to rm container: %s", c.Id)
					if _, _, err := daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
						glog.V(1).Infof("Error to rm container: %s", err.Error())
					}
				}
				daemon.DeletePodContainerFromDB(mypod.Id)
				daemon.DeleteVolumeId(mypod.Id)
				break
			default:
				break
			}
		}
		return true
	}

	return false
}
