package daemon

import (
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/Unknwon/goconfig"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/hyperhq/hyper/engine"
	dockertypes "github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/lib/docker/graph"
	"github.com/hyperhq/hyper/lib/portallocator"
	apiserver "github.com/hyperhq/hyper/server"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	DefaultResourcePath string = "/var/run/hyper/Pods"
)

type DockerInterface interface {
	SendCmdCreate(name, image string, cmds []string, config interface{}) ([]byte, int, error)
	SendCmdDelete(arg ...string) ([]byte, int, error)
	SendCmdInfo(args ...string) (*dockertypes.Info, error)
	SendCmdImages(all string) ([]*dockertypes.Image, error)
	GetContainerInfo(args ...string) (*dockertypes.ContainerJSONRaw, error)
	SendCmdPull(image string, config *graph.ImagePullConfig) ([]byte, int, error)
	SendCmdAuth(body io.ReadCloser) (string, error)
	SendCmdPush(remote string, ipconfig *graph.ImagePushConfig) error
	SendImageDelete(args ...string) ([]dockertypes.ImageDelete, error)
	SendImageBuild(image string, context io.ReadCloser) ([]byte, int, error)
	SendContainerCommit(args ...string) ([]byte, int, error)
	SendContainerRename(oName, nName string) error
	Shutdown() error
	Setup() error
}

type Daemon struct {
	ID          string
	db          *leveldb.DB
	eng         *engine.Engine
	DockerCli   DockerInterface
	PodList     map[string]*hypervisor.PodStatus
	PodsMutex   *sync.RWMutex
	VmList      map[string]*hypervisor.Vm
	Kernel      string
	Initrd      string
	Bios        string
	Cbfs        string
	VboxImage   string
	BridgeIface string
	BridgeIP    string
	Host        string
	Storage     Storage
	Hypervisor  string
	DefaultLog  *pod.PodLogConfig
}

// Install installs daemon capabilities to eng.
func (daemon *Daemon) Install(eng *engine.Engine) error {
	// Now, we just install a command 'info' to set/get the information of the docker and Hyper daemon
	for name, method := range map[string]engine.Handler{
		"auth":              daemon.CmdAuth,
		"info":              daemon.CmdInfo,
		"version":           daemon.CmdVersion,
		"create":            daemon.CmdCreate,
		"pull":              daemon.CmdPull,
		"build":             daemon.CmdBuild,
		"commit":            daemon.CmdCommit,
		"rename":            daemon.CmdRename,
		"push":              daemon.CmdPush,
		"podCreate":         daemon.CmdPodCreate,
		"podStart":          daemon.CmdPodStart,
		"podInfo":           daemon.CmdPodInfo,
		"containerInfo":     daemon.CmdContainerInfo,
		"podRm":             daemon.CmdPodRm,
		"podRun":            daemon.CmdPodRun,
		"podStop":           daemon.CmdPodStop,
		"vmCreate":          daemon.CmdVmCreate,
		"vmKill":            daemon.CmdVmKill,
		"list":              daemon.CmdList,
		"exec":              daemon.CmdExec,
		"attach":            daemon.CmdAttach,
		"tty":               daemon.CmdTty,
		"serviceAdd":        daemon.AddService,
		"serviceList":       daemon.GetServices,
		"serviceUpdate":     daemon.UpdateService,
		"serviceDelete":     daemon.DeleteService,
		"serveapi":          apiserver.ServeApi,
		"acceptconnections": apiserver.AcceptConnections,

		"images":       daemon.CmdImages,
		"imagesremove": daemon.CmdImagesRemove,
	} {
		glog.V(3).Infof("Engine Register: name= %s", name)
		if err := eng.Register(name, method); err != nil {
			return err
		}
	}
	return nil
}

func (daemon *Daemon) Restore() error {
	if daemon.GetPodNum() == 0 {
		return nil
	}

	podList := map[string]string{}

	iter := daemon.db.NewIterator(util.BytesPrefix([]byte("pod-")), nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()
		if strings.Contains(string(key), "pod-container-") {
			glog.V(1).Infof(string(value))
			continue
		}
		glog.V(1).Infof("Get the pod item, pod is %s!", key)
		err := daemon.db.Delete(key, nil)
		if err != nil {
			return err
		}
		podList[string(key)[4:]] = string(value)
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return err
	}
	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodsMutex.Unlock()
	for k, v := range podList {
		err = daemon.CreatePod(k, v, nil, false)
		if err != nil {
			glog.Warningf("Got a unexpected error, %s", err.Error())
			continue
		}
		vmId, err := daemon.GetVmByPod(k)
		if err != nil {
			glog.V(1).Info(err.Error(), " for ", k)
			continue
		}
		daemon.PodList[k].Vm = string(vmId)
	}

	// associate all VMs
	daemon.AssociateAllVms()
	return nil
}

func NewDaemon(eng *engine.Engine) (*Daemon, error) {
	daemon, err := NewDaemonFromDirectory(eng)
	if err != nil {
		return nil, err
	}
	return daemon, nil
}

func NewDaemonFromDirectory(eng *engine.Engine) (*Daemon, error) {
	// register portallocator release on shutdown
	eng.OnShutdown(func() {
		if err := portallocator.ReleaseAll(); err != nil {
			glog.Errorf("portallocator.ReleaseAll(): %s", err.Error())
		}
	})
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("The Hyper daemon needs to be run as root")
	}
	if err := checkKernel(); err != nil {
		return nil, err
	}

	cfg, err := goconfig.LoadConfigFile(eng.Config)
	if err != nil {
		glog.Errorf("Read config file (%s) failed, %s", eng.Config, err.Error())
		return nil, err
	}
	kernel, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Kernel")
	initrd, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Initrd")
	glog.V(0).Infof("The config: kernel=%s, initrd=%s", kernel, initrd)
	vboxImage, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Vbox")
	glog.V(0).Infof("The config: vbox image=%s", vboxImage)
	biface, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Bridge")
	bridgeip, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "BridgeIP")
	glog.V(0).Infof("The config: bridge=%s, ip=%s", biface, bridgeip)
	bios, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Bios")
	cbfs, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Cbfs")
	glog.V(0).Infof("The config: bios=%s, cbfs=%s", bios, cbfs)
	host, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Host")

	var tempdir = path.Join(utils.HYPER_ROOT, "run")
	os.Setenv("TMPDIR", tempdir)
	if err := os.MkdirAll(tempdir, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}

	var realRoot = path.Join(utils.HYPER_ROOT, "lib")
	// Create the root directory if it doesn't exists
	if err := os.MkdirAll(realRoot, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}

	var (
		db_file = fmt.Sprintf("%s/hyper.db", realRoot)
	)
	db, err := leveldb.OpenFile(db_file, nil)
	if err != nil {
		glog.Errorf("open leveldb file failed, %s", err.Error())
		return nil, err
	}
	dockerCli, err1 := NewDocker()
	if err1 != nil {
		glog.Errorf(err1.Error())
		return nil, err1
	}
	pList := map[string]*hypervisor.PodStatus{}
	vList := map[string]*hypervisor.Vm{}
	daemon := &Daemon{
		ID:          fmt.Sprintf("%d", os.Getpid()),
		db:          db,
		eng:         eng,
		Kernel:      kernel,
		Initrd:      initrd,
		Bios:        bios,
		Cbfs:        cbfs,
		VboxImage:   vboxImage,
		DockerCli:   dockerCli,
		PodList:     pList,
		PodsMutex:   new(sync.RWMutex),
		VmList:      vList,
		Host:        host,
		BridgeIP:    bridgeip,
		BridgeIface: biface,
	}

	// Get the docker daemon info
	sysinfo, err := dockerCli.SendCmdInfo()
	if err != nil {
		return nil, err
	}
	stor, err := StorageFactory(sysinfo)
	if err != nil {
		return nil, err
	}
	daemon.Storage = stor
	daemon.Storage.Init()
	eng.OnShutdown(func() {
		if err := daemon.shutdown(); err != nil {
			glog.Errorf("Error during daemon.shutdown(): %v", err)
		}
	})

	return daemon, nil
}

func (daemon *Daemon) DefaultLogCfg(driver string, cfg map[string]string) {
	if driver == "" {
		driver = jsonfilelog.Name
	}

	daemon.DefaultLog = &pod.PodLogConfig{
		Type:   driver,
		Config: cfg,
	}
}

func (daemon *Daemon) GetPodNum() int64 {
	iter := daemon.db.NewIterator(util.BytesPrefix([]byte("pod-")), nil)
	var i int64 = 0
	for iter.Next() {
		key := iter.Key()
		if strings.Contains(string(key), "pod-container-") {
			continue
		}
		i++
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return 0
	}
	return i
}

func (daemon *Daemon) GetRunningPodNum() int64 {
	var num int64 = 0
	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	for _, v := range daemon.PodList {
		if v.Status == types.S_POD_RUNNING {
			num++
		}
	}
	daemon.PodsMutex.RUnlock()
	glog.V(2).Infof("unlock read of PodList")
	return num
}

func (daemon *Daemon) WritePodToDB(podName string, podData []byte) error {
	key := fmt.Sprintf("pod-%s", podName)
	_, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		err = daemon.db.Put([]byte(key), podData, nil)
		if err != nil {
			return err
		}
	} else {
		err = daemon.db.Delete([]byte(key), nil)
		if err != nil {
			return err
		}
		err = daemon.db.Put([]byte(key), podData, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (daemon *Daemon) GetPod(podId, podArgs string, autoremove bool) (*Pod, error) {
	var (
		ps      *hypervisor.PodStatus
		spec    *pod.UserPod
		podData []byte
		err     error
	)
	if podArgs == "" {
		var ok bool
		if ps, ok = daemon.PodList[podId]; !ok {
			return nil, fmt.Errorf("Can not find the POD instance of %s", podId)
		}

		podData, err = daemon.GetPodByName(podId)
		if err != nil {
			return nil, err
		}
	} else {
		podData = []byte(podArgs)

		if err = daemon.CreatePod(podId, podArgs, nil, autoremove); err != nil {
			glog.Error(err.Error())
			return nil, err
		}

		ps = daemon.PodList[podId]
	}

	spec, err = daemon.ProcessPodBytes(podData, podId)
	if err != nil {
		return nil, err
	}

	pod := &Pod{
		id:     podId,
		status: ps,
		spec:   spec,
		vm:     nil,
	}

	return pod, nil
}

func (daemon *Daemon) GetPodByName(podName string) ([]byte, error) {
	key := fmt.Sprintf("pod-%s", podName)
	data, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		return []byte(""), err
	}
	return data, nil
}

func (daemon *Daemon) DeletePodFromDB(podName string) error {
	key := fmt.Sprintf("pod-%s", podName)
	err := daemon.db.Delete([]byte(key), nil)
	if err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) SetVolumeId(podId, volName, dev_id string) error {
	key := fmt.Sprintf("vol-%s-%s", podId, dev_id)
	err := daemon.db.Put([]byte(key), []byte(fmt.Sprintf("%s:%s", volName, dev_id)), nil)
	if err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) GetMaxDeviceId() (int, error) {
	iter := daemon.db.NewIterator(util.BytesPrefix([]byte("vol-")), nil)
	maxId := 1
	for iter.Next() {
		value := iter.Value()
		fields := strings.Split(string(value), ":")
		id, _ := strconv.Atoi(fields[1])
		if id > maxId {
			maxId = id
		}
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return -1, err
	}
	return maxId, nil
}

func (daemon *Daemon) GetVolumeId(podId, volName string) (int, error) {
	dev_id := 0
	key := fmt.Sprintf("vol-%s", podId)
	iter := daemon.db.NewIterator(util.BytesPrefix([]byte(key)), nil)
	for iter.Next() {
		value := iter.Value()
		fields := strings.Split(string(value), ":")
		if fields[0] == volName {
			dev_id, _ = strconv.Atoi(fields[1])
		}
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return -1, err
	}
	return dev_id, nil
}

func (daemon *Daemon) DeleteVolumeId(podId string) error {
	key := fmt.Sprintf("vol-%s", podId)
	iter := daemon.db.NewIterator(util.BytesPrefix([]byte(key)), nil)
	for iter.Next() {
		value := iter.Key()
		daemon.Storage.RemoveVolume(podId, iter.Value())
		err := daemon.db.Delete(value, nil)
		if err != nil {
			return err
		}
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return err
	}
	return nil
}
func (daemon *Daemon) WritePodAndContainers(podId string) error {
	key := fmt.Sprintf("pod-container-%s", podId)
	value := ""
	for _, c := range daemon.PodList[podId].Containers {
		if value == "" {
			value = c.Id
		} else {
			value = value + ":" + c.Id
		}
	}
	err := daemon.db.Put([]byte(key), []byte(value), nil)
	if err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) GetPodContainersByName(podName string) ([]string, error) {
	key := fmt.Sprintf("pod-container-%s", podName)
	data, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		return nil, err
	}
	containers := strings.Split(string(data), ":")
	return containers, nil
}

func (daemon *Daemon) DeletePodContainerFromDB(podName string) error {
	key := fmt.Sprintf("pod-container-%s", podName)
	err := daemon.db.Delete([]byte(key), nil)
	if err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) GetVmByPod(podId string) (string, error) {
	key := fmt.Sprintf("vm-%s", podId)
	data, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (daemon *Daemon) UpdateVmByPod(podId, vmId string) error {
	glog.V(1).Infof("Add or Update the VM info for pod(%s)", podId)
	key := fmt.Sprintf("vm-%s", podId)
	_, err := daemon.db.Get([]byte(key), nil)
	if err == nil {
		err = daemon.db.Delete([]byte(key), nil)
		if err != nil {
			return err
		}
		err = daemon.db.Put([]byte(key), []byte(vmId), nil)
		if err != nil {
			return err
		}
	} else {
		err = daemon.db.Put([]byte(key), []byte(vmId), nil)
		if err != nil {
			return err
		}
	}
	glog.V(1).Infof("success to add or  update the VM info for pod(%s)", podId)
	return nil
}

func (daemon *Daemon) DeleteVmByPod(podId string) error {
	key := fmt.Sprintf("vm-%s", podId)
	vmId, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		return err
	}
	err = daemon.db.Delete([]byte(key), nil)
	if err != nil {
		return err
	}
	if err = daemon.DeleteVmData(string(vmId)); err != nil {
		return err
	}
	glog.V(1).Infof("success to delete the VM info for pod(%s)", podId)
	return nil
}

func (daemon *Daemon) GetPodVmByName(podName string) (string, error) {
	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodsMutex.RUnlock()
	pod := daemon.PodList[podName]
	if pod == nil {
		return "", fmt.Errorf("Not found VM for pod(%s)", podName)
	}
	return pod.Vm, nil
}

func (daemon *Daemon) GetPodByContainer(containerId string) (string, error) {
	var c *hypervisor.Container = nil

	daemon.PodsMutex.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodsMutex.RUnlock()
	for _, p := range daemon.PodList {
		for _, c = range p.Containers {
			if c.Id == containerId {
				return p.Id, nil
			}
		}
	}

	return "", fmt.Errorf("Can not find that container!")
}

func (daemon *Daemon) AddPod(pod *hypervisor.PodStatus) {
	daemon.PodList[pod.Id] = pod
}

func (daemon *Daemon) RemovePod(podId string) {
	delete(daemon.PodList, podId)
}

func (daemon *Daemon) AddVm(vm *hypervisor.Vm) {
	daemon.VmList[vm.Id] = vm
}

func (daemon *Daemon) RemoveVm(vmId string) {
	delete(daemon.VmList, vmId)
}

func (daemon *Daemon) UpdateVmData(vmId string, data []byte) error {
	key := fmt.Sprintf("vmdata-%s", vmId)
	_, err := daemon.db.Get([]byte(key), nil)
	if err == nil {
		err = daemon.db.Delete([]byte(key), nil)
		if err != nil {
			return err
		}
		err = daemon.db.Put([]byte(key), data, nil)
		if err != nil {
			return err
		}
	}
	if err != nil && strings.Contains(err.Error(), "not found") {
		err = daemon.db.Put([]byte(key), data, nil)
		if err != nil {
			return err
		}
	}
	return err
}

func (daemon *Daemon) GetVmData(vmId string) ([]byte, error) {
	key := fmt.Sprintf("vmdata-%s", vmId)
	data, err := daemon.db.Get([]byte(key), nil)
	if err != nil {
		return []byte(""), err
	}
	return data, nil
}

func (daemon *Daemon) DeleteVmData(vmId string) error {
	key := fmt.Sprintf("vmdata-%s", vmId)
	err := daemon.db.Delete([]byte(key), nil)
	if err != nil {
		return err
	}
	return nil
}

func (daemon *Daemon) DestroyAllVm() error {
	glog.V(0).Info("The daemon will stop all pod")
	daemon.PodsMutex.Lock()
	glog.V(2).Infof("lock PodList")
	for _, pod := range daemon.PodList {
		daemon.StopPod(pod.Id, "yes")
	}
	daemon.PodsMutex.Unlock()
	glog.V(2).Infof("unlock PodList")
	iter := daemon.db.NewIterator(util.BytesPrefix([]byte("vm-")), nil)
	for iter.Next() {
		key := iter.Key()
		daemon.db.Delete(key, nil)
	}
	iter.Release()
	err := iter.Error()
	return err
}

func (daemon *Daemon) DestroyAndKeepVm() error {
	for i := 0; i < 3; i++ {
		code, err := daemon.ReleaseAllVms()
		if err != nil && code == types.E_BUSY {
			continue
		} else {
			return err
		}
	}
	return nil
}

func (daemon *Daemon) shutdown() error {
	glog.V(0).Info("The daemon will be shutdown")
	glog.V(0).Info("Shutdown all VMs")
	for vm := range daemon.VmList {
		daemon.KillVm(vm)
	}
	daemon.db.Close()
	glog.Flush()
	return nil
}

func NewDocker() (DockerInterface, error) {
	return NewDockerImpl()
}

var NewDockerImpl = func() (DockerInterface, error) {
	return nil, fmt.Errorf("no docker create function")
}

// Now, the daemon can be ran for any linux kernel
func checkKernel() error {
	return nil
}
