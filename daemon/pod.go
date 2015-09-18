package daemon

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"runtime"
	"strings"

	"github.com/hyperhq/hyper/engine"
	dockertypes "github.com/hyperhq/hyper/lib/docker/api/types"
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

	podId := job.Args[0]
	vmId := job.Args[1]

	glog.Infof("pod:%s, vm:%s", podId, vmId)
	// Do the status check for the given pod
	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()
	if _, ok := daemon.PodList[podId]; !ok {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode() && vmId == ""

	code, cause, err := daemon.StartPod(podId, "", vmId, nil, lazy, false, types.VM_KEEP_NONE)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

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
	var autoremove bool = false
	podArgs := job.Args[0]
	if job.Args[1] == "yes" {
		autoremove = true
	}

	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))

	glog.Info(podArgs)

	var lazy bool = hypervisor.HDriver.SupportLazyMode()

	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()
	code, cause, err := daemon.StartPod(podId, podArgs, "", nil, lazy, autoremove, types.VM_KEEP_NONE)
	if err != nil {
		glog.Error(err.Error())
		return err
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
	userPod, err = pod.ProcessPodBytes([]byte(podArgs))
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
		glog.V(1).Info("Process the Containers section in POD SPEC\n")
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
		uid               string
		gid               string
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
		rootPath = "/var/lib/docker/devicemapper"
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
			file, ok := files[f.Filename]
			if !ok {
				continue
			}
			var fromFile = "/tmp/" + file.Name
			defer os.RemoveAll(fromFile)
			if file.Uri != "" {
				err = utils.DownloadFile(file.Uri, fromFile)
				if err != nil {
					return nil, err
				}
			} else if file.Contents != "" {
				err = ioutil.WriteFile(fromFile, []byte(file.Contents), 0666)
				if err != nil {
					return nil, err
				}
			} else {
				continue
			}
			// we need to decode the content
			fi, err := os.Open(fromFile)
			if err != nil {
				return nil, err
			}
			defer fi.Close()
			fileContent, err := ioutil.ReadAll(fi)
			if err != nil {
				return nil, err
			}
			if file.Encoding == "base64" {
				newContent, err := utils.Base64Decode(string(fileContent))
				if err != nil {
					return nil, err
				}
				err = ioutil.WriteFile(fromFile, []byte(newContent), 0666)
				if err != nil {
					return nil, err
				}
			} else {
				err = ioutil.WriteFile(fromFile, []byte(file.Contents), 0666)
				if err != nil {
					return nil, err
				}
			}
			// get the uid and gid for that attached file
			fileUser := f.User

			u, err := user.Current()
			if err != nil {
				glog.Error("got error when get current user ", err.Error())
				return nil, err
			}

			if fileUser != "" {
				u, err = user.Lookup(fileUser)
				if err != nil {
					glog.Error("got error when lookup user ", err.Error())
					return nil, err
				}
			}

			uid = u.Uid
			gid = u.Gid

			if storageDriver == "devicemapper" {
				err := dm.AttachFiles(c.Id, devPrefix, fromFile, targetPath, rootPath, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
					return nil, err
				}
			} else if storageDriver == "aufs" {
				err := aufs.AttachFiles(c.Id, fromFile, targetPath, sharedDir, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
					return nil, err
				}
			} else if storageDriver == "overlay" {
				err := overlay.AttachFiles(c.Id, fromFile, targetPath, sharedDir, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
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
				glog.Error("DeviceID is %d", dev_id)
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

		if runtime.GOOS == "linux" {
			if err := os.MkdirAll(targetDir, 0755); err != nil && !os.IsExist(err) {
				glog.Errorf("error to create dir %s for volume %s", targetDir, v.Name)
				return nil, err
			}
		}

		if err := utils.Mount(v.Source, targetDir, "dir", flags, "--bind"); err != nil {
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

func (daemon *Daemon) StartPod(podId, podArgs, vmId string, config interface{}, lazy, autoremove bool, keep int) (int, string, error) {
	var (
		podData []byte
		err     error
		mypod   *hypervisor.Pod
		vm      *hypervisor.Vm = nil
	)

	if podArgs == "" {
		var ok bool
		mypod, ok = daemon.PodList[podId]
		if !ok {
			return -1, "", fmt.Errorf("Can not find the POD instance of %s", podId)
		}

		podData, err = daemon.GetPodByName(podId)
		if err != nil {
			return -1, "", err
		}
	} else {
		podData = []byte(podArgs)

		if err := daemon.CreatePod(podId, podArgs, nil, autoremove); err != nil {
			glog.Error(err.Error())
			return -1, "", err
		}

		mypod = daemon.PodList[podId]
	}

	userPod, err := pod.ProcessPodBytes(podData)
	if err != nil {
		return -1, "", err
	}

	defer func() {
		if vm != nil && err != nil && vmId == "" {
			daemon.KillVm(vm.Id)
		}
	}()

	if vmId == "" {
		glog.V(1).Infof("The config: kernel=%s, initrd=%s", daemon.Kernel, daemon.Initrd)
		var (
			cpu = 1
			mem = 128
		)

		if userPod.Resource.Vcpu > 0 {
			cpu = userPod.Resource.Vcpu
		}

		if userPod.Resource.Memory > 0 {
			mem = userPod.Resource.Memory
		}

		b := &hypervisor.BootConfig{
			CPU:    cpu,
			Memory: mem,
			Kernel: daemon.Kernel,
			Initrd: daemon.Initrd,
			Bios:   daemon.Bios,
			Cbfs:   daemon.Cbfs,
			Vbox:   daemon.VboxImage,
		}

		vm = daemon.NewVm("", cpu, mem, lazy, keep)

		err = vm.Launch(b)
		if err != nil {
			return -1, "", err
		}

		daemon.AddVm(vm)
	} else {
		var ok bool
		vm, ok = daemon.VmList[vmId]
		if !ok {
			err = fmt.Errorf("The VM %s doesn't exist", vmId)
			return -1, "", err
		}
		/* FIXME: check if any pod is running on this vm? */
		glog.Infof("find vm:%s", vm.Id)
		if userPod.Resource.Vcpu != vm.Cpu {
			err = fmt.Errorf("The new pod's cpu setting is different with the VM's cpu")
			return -1, "", err
		}

		if userPod.Resource.Memory != vm.Mem {
			err = fmt.Errorf("The new pod's memory setting is different with the VM's memory")
			return -1, "", err
		}
	}

	fmt.Printf("POD id is %s\n", podId)

	containerInfoList, volumeInfoList, err := daemon.PreparePod(mypod, userPod, vm.Id)
	if err != nil {
		return -1, "", err
	}

	vmResponse := vm.StartPod(mypod, userPod, containerInfoList, volumeInfoList)
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
	_, _, err = daemon.StartPod(mypod.Id, string(podData), "", nil, lazy, false, types.VM_KEEP_NONE)
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
