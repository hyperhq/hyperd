package daemon

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strings"
	"sync"
	"syscall"

	"hyper/docker"
	"hyper/engine"
	"hyper/hypervisor"
	"hyper/lib/glog"
	"hyper/pod"
	"hyper/storage/aufs"
	dm "hyper/storage/devicemapper"
	"hyper/storage/overlay"
	"hyper/types"
	"hyper/utils"
)

func (daemon *Daemon) CmdPodCreate(job *engine.Job) error {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}
	podArgs := job.Args[0]

	wg := new(sync.WaitGroup)
	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	err := daemon.CreatePod(podArgs, podId, wg)
	if err != nil {
		return err
	}
	if err := daemon.WritePodAndContainers(podId); err != nil {
		glog.V(1).Info("Found an error while saveing the Containers info")
		return err
	}

	// Prepare the qemu status to client
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

	glog.Info("pod:%s, vm:%s", podId, vmId)
	// Do the status check for the given pod
	if pod, ok := daemon.podList[podId]; ok {
		if pod.Status == types.S_POD_RUNNING {
			return fmt.Errorf("The pod(%s) is running, can not start it", podId)
		} else {
			if pod.Type == "kubernetes" && pod.Status != types.S_POD_CREATED {
				return fmt.Errorf("The pod(%s) is finished with kubernetes type, can not start it again", podId)
			}
		}
	} else {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}
	data, err := daemon.GetPodByName(podId)
	if err != nil {
		return err
	}
	userPod, err := pod.ProcessPodBytes(data)
	if err != nil {
		return err
	}
	if vmId == "" {
		vmId = fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
	} else {
		if _, ok := daemon.vmList[vmId]; !ok {
			return fmt.Errorf("The VM %s doesn't exist", vmId)
		}
		if userPod.Resource.Vcpu != daemon.vmList[vmId].Cpu {
			return fmt.Errorf("The new pod's cpu setting is different the current VM's cpu")
		}
		if userPod.Resource.Memory != daemon.vmList[vmId].Mem {
			return fmt.Errorf("The new pod's memory setting is different the current VM's memory")
		}
	}

	code, cause, err := daemon.StartPod(podId, vmId, "")
	if err != nil {
		daemon.KillVm(vmId)
		glog.Error(err.Error())
		return err
	}

	vm := &Vm{
		Id:     vmId,
		Pod:    daemon.podList[podId],
		Status: types.S_VM_ASSOCIATED,
		Cpu:    userPod.Resource.Vcpu,
		Mem:    userPod.Resource.Memory,
	}
	daemon.podList[podId].Vm = vmId
	daemon.AddVm(vm)

	// Prepare the qemu status to client
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
	podArgs := job.Args[0]

	vmId := fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))

	glog.Info(podArgs)

	code, cause, err := daemon.StartPod(podId, vmId, podArgs)
	if err != nil {
		daemon.KillVm(vmId)
		glog.Error(err.Error())
		return err
	}
	if err := daemon.WritePodAndContainers(podId); err != nil {
		glog.V(1).Info("Found an error while saveing the Containers info")
		return err
	}
	data, err := daemon.GetPodByName(podId)
	if err != nil {
		return err
	}
	userPod, err := pod.ProcessPodBytes(data)
	if err != nil {
		return err
	}

	vm := &Vm{
		Id:     vmId,
		Pod:    daemon.podList[podId],
		Status: types.S_VM_ASSOCIATED,
		Cpu:    userPod.Resource.Vcpu,
		Mem:    userPod.Resource.Memory,
	}
	daemon.podList[podId].Vm = vmId
	daemon.AddVm(vm)

	// Prepare the qemu status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CreatePod(podArgs, podId string, wg *sync.WaitGroup) error {
	userPod, err := pod.ProcessPodBytes([]byte(podArgs))
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return err
	}
	if err := userPod.Validate(); err != nil {
		return err
	}
	// store the UserPod into the db
	if err := daemon.WritePodToDB(podId, []byte(podArgs)); err != nil {
		glog.V(1).Info("Found an error while saveing the POD file")
		return err
	}
	containerIds, err := daemon.GetPodContainersByName(podId)
	if err != nil {
		glog.V(1).Info(err.Error())
	}
	if containerIds != nil {
		for _, id := range containerIds {
			daemon.SetPodByContainer(id, podId, "", "", []string{}, types.S_POD_CREATED)
		}
	} else {
		// Process the 'Containers' section
		glog.V(1).Info("Process the Containers section in POD SPEC\n")
		for _, c := range userPod.Containers {
			imgName := c.Image
			body, _, err := daemon.dockerCli.SendCmdCreate(imgName)
			if err != nil {
				glog.Error(err.Error())
				daemon.DeletePodFromDB(podId)
				return err
			}
			out := engine.NewOutput()
			remoteInfo, err := out.AddEnv()
			if err != nil {
				daemon.DeletePodFromDB(podId)
				return err
			}
			if _, err := out.Write(body); err != nil {
				daemon.DeletePodFromDB(podId)
				return fmt.Errorf("Error while reading remote info!\n")
			}
			out.Close()

			containerId := remoteInfo.Get("Id")
			daemon.SetPodByContainer(containerId, podId, "", "", []string{}, types.S_POD_CREATED)
		}
	}
	containers := []*Container{}
	for _, v := range daemon.containerList {
		if v.PodId == podId {
			containers = append(containers, v)
		}
	}
	mypod := &Pod{
		Id:            podId,
		Name:          userPod.Name,
		Vm:            "",
		Wg:            wg,
		Containers:    containers,
		Status:        types.S_POD_CREATED,
		Type:          userPod.Type,
		RestartPolicy: userPod.Containers[0].RestartPolicy,
	}
	daemon.AddPod(mypod)

	return nil
}

func (daemon *Daemon) StartPod(podId, vmId, podArgs string) (int, string, error) {
	var (
		fstype            string
		poolName          string
		volPoolName       string
		devPrefix         string
		storageDriver     string
		rootPath          string
		devFullName       string
		rootfs            string
		containerInfoList = []*hypervisor.ContainerInfo{}
		volumuInfoList    = []*hypervisor.VolumeInfo{}
		cli               = daemon.dockerCli
		qemuPodEvent      = make(chan hypervisor.VmEvent, 128)
		qemuStatus        = make(chan *types.QemuResponse, 128)
		subQemuStatus     = make(chan *types.QemuResponse, 128)
		sharedDir         = path.Join(hypervisor.BaseDir, vmId, hypervisor.ShareDirTag)
		podData           []byte
		mypod             *Pod
		wg                *sync.WaitGroup
		err               error
		uid               string
		gid               string
	)
	if podArgs == "" {
		mypod = daemon.podList[podId]
		if mypod == nil {
			return -1, "", fmt.Errorf("Can not find the POD instance of %s", podId)
		}
		podData, err = daemon.GetPodByName(podId)
		if err != nil {
			return -1, "", err
		}
		wg = mypod.Wg
	} else {
		podData = []byte(podArgs)
	}
	userPod, err := pod.ProcessPodBytes(podData)
	if err != nil {
		return -1, "", err
	}

	vm := daemon.vmList[vmId]
	if vm == nil {
		glog.V(1).Infof("The config: kernel=%s, initrd=%s", daemon.kernel, daemon.initrd)
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
			Kernel: daemon.kernel,
			Initrd: daemon.initrd,
			Bios:   daemon.bios,
			Cbfs:   daemon.cbfs,
		}
		go hypervisor.VmLoop(hypervisorDriver, vmId, qemuPodEvent, qemuStatus, b)
		if err := daemon.SetQemuChan(vmId, qemuPodEvent, qemuStatus, subQemuStatus); err != nil {
			glog.V(1).Infof("SetQemuChan error: %s", err.Error())
			return -1, "", err
		}

	} else {
		ret1, ret2, ret3, err := daemon.GetQemuChan(vmId)
		if err != nil {
			return -1, "", err
		}
		qemuPodEvent = ret1.(chan hypervisor.VmEvent)
		qemuStatus = ret2.(chan *types.QemuResponse)
		subQemuStatus = ret3.(chan *types.QemuResponse)
	}
	if podArgs != "" {
		wg = new(sync.WaitGroup)
		if err := daemon.CreatePod(podArgs, podId, wg); err != nil {
			glog.Error(err.Error())
			return -1, "", err
		}
		mypod = daemon.podList[podId]
	}

	storageDriver = daemon.Storage.StorageType
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
	}

	// Process the 'Files' section
	files := make(map[string](pod.UserFile))
	for _, v := range userPod.Files {
		files[v.Name] = v
	}

	for i, c := range mypod.Containers {
		var jsonResponse *docker.ConfigJSON
		if jsonResponse, err = cli.GetContainerInfo(c.Id); err != nil {
			glog.Error("got error when get container Info ", err.Error())
			return -1, "", err
		}

		if storageDriver == "devicemapper" {
			if err := dm.CreateNewDevice(c.Id, devPrefix, rootPath); err != nil {
				return -1, "", err
			}
			devFullName, err = dm.MountContainerToSharedDir(c.Id, sharedDir, devPrefix)
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return -1, "", err
			}
			fstype, err = dm.ProbeFsType(devFullName)
			if err != nil {
				fstype = "ext4"
			}
		} else if storageDriver == "aufs" {
			devFullName, err = aufs.MountContainerToSharedDir(c.Id, rootPath, sharedDir, "")
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return -1, "", err
			}
			devFullName = "/" + c.Id + "/rootfs"
		} else if storageDriver == "overlay" {
			devFullName, err = overlay.MountContainerToSharedDir(c.Id, rootPath, sharedDir, "")
			if err != nil {
				glog.Error("got error when mount container to share dir ", err.Error())
				return -1, "", err
			}
			devFullName = "/" + c.Id + "/rootfs"
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
					return -1, "", err
				}
			} else if file.Contents != "" {
				err = ioutil.WriteFile(fromFile, []byte(file.Contents), 0666)
				if err != nil {
					return -1, "", err
				}
			} else {
				continue
			}
			// we need to decode the content
			fi, err := os.Open(fromFile)
			if err != nil {
				return -1, "", err
			}
			defer fi.Close()
			fileContent, err := ioutil.ReadAll(fi)
			if err != nil {
				return -1, "", err
			}
			if file.Encoding == "base64" {
				newContent, err := utils.Base64Decode(string(fileContent))
				if err != nil {
					return -1, "", err
				}
				err = ioutil.WriteFile(fromFile, []byte(newContent), 0666)
				if err != nil {
					return -1, "", err
				}
			} else {
				err = ioutil.WriteFile(fromFile, []byte(file.Contents), 0666)
				if err != nil {
					return -1, "", err
				}
			}
			// get the uid and gid for that attached file
			fileUser := f.User
			fileGroup := f.Group
			u, _ := user.Current()
			if fileUser == "" {
				uid = u.Uid
			} else {
				u, _ = user.Lookup(fileUser)
				uid = u.Uid
				gid = u.Gid
			}
			if fileGroup == "" {
				gid = u.Gid
			}

			if storageDriver == "devicemapper" {
				err := dm.AttachFiles(c.Id, devPrefix, fromFile, targetPath, rootPath, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
					return -1, "", err
				}
			} else if storageDriver == "aufs" {
				err := aufs.AttachFiles(c.Id, fromFile, targetPath, rootPath, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
					return -1, "", err
				}
			} else if storageDriver == "overlay" {
				err := overlay.AttachFiles(c.Id, fromFile, targetPath, rootPath, f.Perm, uid, gid)
				if err != nil {
					glog.Error("got error when attach files ", err.Error())
					return -1, "", err
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
			Entrypoint: jsonResponse.Config.Entrypoint,
			Cmd:        jsonResponse.Config.Cmd,
			Envs:       env,
		}
		glog.V(1).Infof("Container Info is \n%v", containerInfo)
		containerInfoList = append(containerInfoList, containerInfo)
		glog.V(1).Infof("container %d created %s, workdir %s, env: %v", i, c.Id, jsonResponse.Config.WorkingDir, env)
	}

	// Process the 'Volumes' section
	for _, v := range userPod.Volumes {
		if v.Source == "" {
			if storageDriver == "devicemapper" {
				volName := fmt.Sprintf("%s-%s-%s", volPoolName, podId, v.Name)
				dev_id, _ := daemon.GetVolumeId(podId, volName)
				glog.Error("DeviceID is %d", dev_id)
				if dev_id < 1 {
					dev_id, _ = daemon.GetMaxDeviceId()
					err := daemon.CreateVolume(podId, volName, fmt.Sprintf("%d", dev_id+1), false)
					if err != nil {
						return -1, "", err
					}
				} else {
					err := daemon.CreateVolume(podId, volName, fmt.Sprintf("%d", dev_id), true)
					if err != nil {
						return -1, "", err
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
				volumuInfoList = append(volumuInfoList, myVol)
				glog.V(1).Infof("volume %s created with dm as %s", v.Name, volName)
				continue

			} else {
				// Make sure the v.Name is given
				v.Source = path.Join("/var/tmp/hyper/", v.Name)
				if _, err := os.Stat(v.Source); err != nil && os.IsNotExist(err) {
					if err := os.MkdirAll(v.Source, os.FileMode(0777)); err != nil {
						return -1, "", err
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
		var flags uintptr = syscall.MS_BIND

		mountSharedDir := pod.RandStr(10, "alpha")
		targetDir := path.Join(sharedDir, mountSharedDir)
		glog.V(1).Infof("trying to bind dir %s to %s", v.Source, targetDir)

		if err := os.MkdirAll(targetDir, 0755); err != nil && !os.IsExist(err) {
			glog.Errorf("error to create dir %s for volume %s", targetDir, v.Name)
			return -1, "", err
		}

		if err := syscall.Mount(v.Source, targetDir, "dir", flags, "--bind"); err != nil {
			glog.Errorf("bind dir %s failed: %s", v.Source, err.Error())
			return -1, "", err
		}
		myVol := &hypervisor.VolumeInfo{
			Name:     v.Name,
			Filepath: mountSharedDir,
			Fstype:   "dir",
			Format:   "",
		}
		glog.V(1).Infof("dir %s is bound to %s", v.Source, targetDir)
		volumuInfoList = append(volumuInfoList, myVol)
	}

	go func(interface{}) {
		for {
			qemuResponse := <-qemuStatus
			subQemuStatus <- qemuResponse
			if qemuResponse.Code == types.E_POD_FINISHED {
				data := qemuResponse.Data.([]uint32)
				daemon.SetPodContainerStatus(podId, data)
				daemon.podList[podId].Vm = ""
			} else if qemuResponse.Code == types.E_VM_SHUTDOWN {
				if daemon.podList[podId].Status == types.S_POD_RUNNING {
					daemon.podList[podId].Status = types.S_POD_SUCCEEDED
					daemon.SetContainerStatus(podId, types.S_POD_SUCCEEDED)
				}
				daemon.podList[podId].Vm = ""
				daemon.RemoveVm(vmId)
				daemon.DeleteQemuChan(vmId)
				mypod = daemon.podList[podId]
				if mypod.Type == "kubernetes" {
					switch mypod.Status {
					case types.S_POD_SUCCEEDED:
						if mypod.RestartPolicy == "always" {
							daemon.RestartPod(mypod)
						} else {
							daemon.DeletePodFromDB(podId)
							for _, c := range daemon.podList[podId].Containers {
								glog.V(1).Infof("Ready to rm container: %s", c.Id)
								if _, _, err = daemon.dockerCli.SendCmdDelete(c.Id); err != nil {
									glog.V(1).Infof("Error to rm container: %s", err.Error())
								}
							}
							//							daemon.RemovePod(podId)
							daemon.DeletePodContainerFromDB(podId)
							daemon.DeleteVolumeId(podId)
						}
						break
					case types.S_POD_FAILED:
						if mypod.RestartPolicy != "never" {
							daemon.RestartPod(mypod)
						} else {
							daemon.DeletePodFromDB(podId)
							for _, c := range daemon.podList[podId].Containers {
								glog.V(1).Infof("Ready to rm container: %s", c.Id)
								if _, _, err = daemon.dockerCli.SendCmdDelete(c.Id); err != nil {
									glog.V(1).Infof("Error to rm container: %s", err.Error())
								}
							}
							//							daemon.RemovePod(podId)
							daemon.DeletePodContainerFromDB(podId)
							daemon.DeleteVolumeId(podId)
						}
						break
					default:
						break
					}
				}
				break
			}
		}
	}(subQemuStatus)

	if daemon.podList[podId].Type == "kubernetes" {
		for _, c := range userPod.Containers {
			c.RestartPolicy = "never"
		}
	}

	fmt.Printf("POD id is %s\n", podId)
	runPodEvent := &hypervisor.RunPodCommand{
		Spec:       userPod,
		Containers: containerInfoList,
		Volumes:    volumuInfoList,
		Wg:         wg,
	}
	qemuPodEvent <- runPodEvent
	daemon.podList[podId].Status = types.S_POD_RUNNING
	// Set the container status to online
	daemon.SetContainerStatus(podId, types.S_POD_RUNNING)

	// wait for the qemu response
	var qemuResponse *types.QemuResponse
	for {
		qemuResponse = <-subQemuStatus
		glog.V(1).Infof("Get the response from QEMU, VM id is %s!", qemuResponse.VmId)
		if qemuResponse.Code == types.E_VM_RUNNING {
			continue
		}
		if qemuResponse.VmId == vmId {
			break
		}
	}
	if qemuResponse.Data == nil {
		return qemuResponse.Code, qemuResponse.Cause, fmt.Errorf("QEMU response data is nil")
	}
	data := qemuResponse.Data.([]byte)
	daemon.UpdateVmData(vmId, data)
	// add or update the Vm info for POD
	if err := daemon.UpdateVmByPod(podId, vmId); err != nil {
		glog.Error(err.Error())
	}

	// XXX we should not close qemuStatus chan, it will be closed in shutdown process
	return qemuResponse.Code, qemuResponse.Cause, nil
}

// The caller must make sure that the restart policy and the status is right to restart
func (daemon *Daemon) RestartPod(mypod *Pod) error {
	// Remove the pod
	// The pod is stopped, the vm is gone
	for _, c := range mypod.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if _, _, err := daemon.dockerCli.SendCmdDelete(c.Id); err != nil {
			glog.V(1).Infof("Error to rm container: %s", err.Error())
		}
	}
	daemon.RemovePod(mypod.Id)
	daemon.DeletePodContainerFromDB(mypod.Id)
	daemon.DeleteVolumeId(mypod.Id)
	podData, err := daemon.GetPodByName(mypod.Id)
	vmId := fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
	// Start the pod
	_, _, err = daemon.StartPod(mypod.Id, vmId, string(podData))
	if err != nil {
		daemon.KillVm(vmId)
		glog.Error(err.Error())
		return err
	}
	if err := daemon.WritePodAndContainers(mypod.Id); err != nil {
		glog.Error("Found an error while saving the Containers info")
		return err
	}
	userPod, err := pod.ProcessPodBytes(podData)
	if err != nil {
		return err
	}

	vm := &Vm{
		Id:     vmId,
		Pod:    daemon.podList[mypod.Id],
		Status: types.S_VM_ASSOCIATED,
		Cpu:    userPod.Resource.Vcpu,
		Mem:    userPod.Resource.Memory,
	}
	daemon.podList[mypod.Id].Vm = vmId
	daemon.AddVm(vm)

	return nil
}

func (daemon *Daemon) CmdPodInfo(job *engine.Job) error {
	if len(job.Args) == 0 {
		return fmt.Errorf("Can not get POD info without POD ID")
	}
	podName := job.Args[0]
	vmId := ""
	// We need to find the VM which running the POD
	pod, ok := daemon.podList[podName]
	if ok {
		vmId = pod.Vm
	}
	glog.V(1).Infof("Process POD %s: VM ID is %s", podName, vmId)
	v := &engine.Env{}
	v.Set("hostname", vmId)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
