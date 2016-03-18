package daemon

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/Unknwon/goconfig"
	docker "github.com/docker/docker/daemon"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/docker/docker/opts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/registry"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	DefaultResourcePath string = "/var/run/hyper/Pods"
)

type Daemon struct {
	*docker.Daemon
	ID          string
	db          *leveldb.DB
	PodList     *PodList
	VmList      map[string]*hypervisor.Vm
	vmCache     VmCache
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

	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()

	for k, v := range podList {
		_, err = daemon.createPodInternal(k, v, false, true)
		if err != nil {
			glog.Warningf("Got a unexpected error, %s", err.Error())
			continue
		}
		vmId, err := daemon.DbGetVmByPod(k)
		if err != nil {
			glog.V(1).Info(err.Error(), " for ", k)
			continue
		}
		p, _ := daemon.PodList.Get(k)
		if err := p.AssociateVm(daemon, string(vmId)); err != nil {
			glog.V(1).Info("Some problem during associate vm %s to pod %s, %v", string(vmId), k, err)
			// continue to next
		}
	}

	return nil
}

func NewDaemon(cfg *goconfig.ConfigFile) (*Daemon, error) {
	daemon, err := NewDaemonFromDirectory(cfg)
	if err != nil {
		return nil, err
	}
	return daemon, nil
}

func NewDaemonFromDirectory(cfg *goconfig.ConfigFile) (*Daemon, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("The Hyper daemon needs to be run as root")
	}
	if err := checkKernel(); err != nil {
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

	daemon := &Daemon{
		ID:          fmt.Sprintf("%d", os.Getpid()),
		db:          db,
		Kernel:      kernel,
		Initrd:      initrd,
		Bios:        bios,
		Cbfs:        cbfs,
		VboxImage:   vboxImage,
		PodList:     NewPodList(),
		VmList:      make(map[string]*hypervisor.Vm),
		Host:        host,
		BridgeIP:    bridgeip,
		BridgeIface: biface,
	}
	daemon.vmCache.daemon = daemon

	daemon.Daemon, err = docker.NewDaemon(dockerCfg, registryCfg)
	if err != nil {
		return nil, err
	}

	// Get the docker daemon info
	sysinfo, err := daemon.Daemon.SystemInfo()
	if err != nil {
		return nil, err
	}
	stor, err := StorageFactory(sysinfo)
	if err != nil {
		return nil, err
	}
	daemon.Storage = stor
	daemon.Storage.Init()

	return daemon, nil
}

var (
	dockerCfg   = &docker.Config{}
	registryCfg = &registry.Service{}
)

func presentInHelp(usage string) string { return usage }
func absentFromHelp(string) string      { return "" }

func InitDockerCfg(mirrors []string, insecureRegistries []string, graphdriver, root string) {
	if dockerCfg.LogConfig.Config == nil {
		dockerCfg.LogConfig.Config = make(map[string]string)
	}

	dockerCfg.LogConfig.Config = make(map[string]string)
	var errhandler flag.ErrorHandling = flag.ContinueOnError
	flags := flag.NewFlagSet("", errhandler)
	dockerCfg.InstallFlags(flags, presentInHelp)

	dockerCfg.GraphDriver = graphdriver
	dockerCfg.Root = root
	dockerCfg.TrustKeyPath = path.Join(root, "keys")

	// disable docker network
	flags.Set("-bridge", "none")
	flags.Set("-iptables", "false")
	flags.Set("-ipmasq", "false")

	// disable log driver
	dockerCfg.LogConfig.Type = "none"

	// debug mode
	if glog.V(3) {
		logrus.SetLevel(logrus.DebugLevel)
	}

	registryOpts := &registry.Options{
		Mirrors:            opts.NewListOpts(nil),
		InsecureRegistries: opts.NewListOpts(nil),
	}
	registryOpts.InstallFlags(flags, absentFromHelp)

	for _, m := range mirrors {
		registryOpts.Mirrors.Set(m)
	}

	for _, ir := range insecureRegistries {
		registryOpts.InsecureRegistries.Set(ir)
	}

	registryCfg = registry.NewService(registryOpts)
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
	return daemon.PodList.CountRunning()
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

// Lock protected
func (daemon *Daemon) GetPod(podId, podArgs string, autoremove bool) (*Pod, error) {
	var (
		pod *Pod
		ok  bool
	)

	if podArgs == "" {
		if pod, ok = daemon.PodList.Get(podId); !ok {
			return nil, fmt.Errorf("Can not find the POD instance of %s", podId)
		}
		return pod, nil
	}

	pod, err := daemon.createPodInternal(podId, podArgs, autoremove, true)
	if err != nil {
		return nil, err
	}

	if err = daemon.AddPod(pod, podArgs); err != nil {
		return nil, err
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
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Cannot find Pod %s to write", podId)
	}

	for _, c := range p.status.Containers {
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

func (daemon *Daemon) DbGetVmByPod(podId string) (string, error) {
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

func (daemon *Daemon) GetVmByPodId(podId string) (string, error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodList.RUnlock()
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return "", fmt.Errorf("Not found Pod %s", podId)
	}
	return pod.status.Vm, nil
}

func (daemon *Daemon) GetPodByContainer(containerId string) (string, error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodList.RUnlock()

	if pod, ok := daemon.PodList.GetByContainerId(containerId); ok {
		return pod.id, nil
	} else {
		return "", fmt.Errorf("Can not find that container!")
	}
}

func (daemon *Daemon) GetPodByContainerIdOrName(name string) (pod *Pod, idx int, err error) {
	daemon.PodList.RLock()
	glog.V(2).Infof("lock read of PodList")
	defer glog.V(2).Infof("unlock read of PodList")
	defer daemon.PodList.RUnlock()

	if pod, idx, ok := daemon.PodList.GetByContainerIdOrName(name); ok {
		return pod, idx, nil
	} else {
		return nil, -1, fmt.Errorf("cannot found container %s", name)
	}
}

func (daemon *Daemon) AddPod(pod *Pod, podArgs string) (err error) {
	// store the UserPod into the db
	if err = daemon.WritePodToDB(pod.id, []byte(podArgs)); err != nil {
		glog.V(1).Info("Found an error while saveing the POD file")
		return
	}
	defer func() {
		if err != nil {
			daemon.DeletePodFromDB(pod.id)
		}
	}()

	daemon.PodList.Put(pod)
	defer func() {
		if err != nil {
			daemon.RemovePod(pod.id)
		}
	}()

	if err = daemon.WritePodAndContainers(pod.id); err != nil {
		glog.V(1).Info("Found an error while saveing the Containers info")
		return
	}

	return nil
}

func (daemon *Daemon) RemovePod(podId string) {
	daemon.PodList.Delete(podId)
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
	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")

	daemon.PodList.Foreach(func(p *Pod) error {
		if _, _, err := daemon.StopPodWithLock(p.id, "yes"); err != nil {
			glog.V(1).Infof("fail to stop %s: %v", p.id, err)
		}
		return nil
	})
	daemon.PodList.Unlock()
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

func (daemon *Daemon) Shutdown() error {
	glog.V(0).Info("The daemon will be shutdown")
	glog.V(0).Info("Shutdown all VMs")
	for vm := range daemon.VmList {
		daemon.KillVm(vm)
	}
	daemon.db.Close()
	glog.Flush()
	return nil
}

// Now, the daemon can be ran for any linux kernel
func checkKernel() error {
	return nil
}
