package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/hyperhq/hyperd/daemon/daemondb"

	"github.com/Unknwon/goconfig"
	docker "github.com/docker/docker/daemon"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/docker/docker/opts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/registry"
	dockerutils "github.com/docker/docker/utils"
	"github.com/golang/glog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/factory"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

var (
	DefaultLogPrefix string = "/var/run/hyper/Pods"
)

type GlobalLogConfig struct {
	*pod.PodLogConfig
	PathPrefix  string
	PodIdInPath bool
}

type Daemon struct {
	*docker.Daemon
	ID          string
	db          *daemondb.DaemonDB
	PodList     *PodList
	VmList      *VmList
	Factory     factory.Factory
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
	DefaultLog  *GlobalLogConfig
}

func (daemon *Daemon) Restore() error {
	if daemon.GetPodNum() == 0 {
		return nil
	}

	ch := daemon.db.GetAllPods()
	if ch == nil {
		estr := "Cannot list pods in leveldb"
		glog.Error(estr)
		return errors.New(estr)
	}

	for {
		item, ok := <-ch
		if !ok {
			break
		}
		if item == nil {
			estr := "error during load pods from leveldb"
			glog.Error(estr)
			return errors.New(estr)
		}

		podId := string(item.K[4:])

		glog.V(1).Infof("reloading pod %s with args %s", podId, string(item.V))

		daemon.db.DeletePod(podId)

		var podSpec apitypes.UserPod
		err := json.Unmarshal(item.V, &podSpec)
		if err != nil {
			return err
		}

		p, err := daemon.createPodInternal(podId, &podSpec, true)
		if err != nil {
			glog.Warningf("Got a unexpected error when creating(load) pod %s, %v", podId, err)
			continue
		}

		if err = daemon.AddPod(p, string(item.V)); err != nil {
			//TODO: remove the created
			glog.Warningf("Got a error duriong insert pod %s, %v", p.Id, err)
			continue
		}

		vmId, err := daemon.db.GetP2V(podId)
		if err != nil {
			glog.V(1).Infof("no existing VM for pod %s: %v", podId, err)
			continue
		}
		if err := p.AssociateVm(daemon, string(vmId)); err != nil {
			glog.V(1).Infof("Some problem during associate vm %s to pod %s, %v", string(vmId), podId, err)
			// continue to next
		}
	}

	if glog.V(3) {
		glog.Infof("%d pod have been loaded", daemon.PodList.CountAll())
		daemon.PodList.Foreach(func(p *Pod) error {
			glog.Infof("container in pod %s status: %v", p.Id, p.Status().Containers)
			glog.Infof("container in pod %s spec: %v", p.Id, p.Spec.Containers)
			return nil
		})
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
	db, err := daemondb.NewDaemonDB(db_file)
	if err != nil {
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
		VmList:      NewVmList(),
		Host:        host,
		BridgeIP:    bridgeip,
		BridgeIface: biface,
	}

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
		dockerutils.EnableDebug()
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

	var (
		logPath   = DefaultLogPrefix
		podInPath = true
	)

	if driver == jsonfilelog.Name {
		if lp, ok := cfg["PodLogPrefix"]; ok {
			logPath = lp
			delete(cfg, "PodLogPrefix")
		}

		if pip, ok := cfg["PodIdInPath"]; ok {
			pip = strings.ToLower(pip)
			if pip == "" || pip == "false" || pip == "no" || pip == "0" {
				podInPath = false
			}
			delete(cfg, "PodIdInPath")
		}
	}

	daemon.DefaultLog = &GlobalLogConfig{
		PodLogConfig: &pod.PodLogConfig{
			Type:   driver,
			Config: cfg,
		},
		PathPrefix:  logPath,
		PodIdInPath: podInPath,
	}
}

func (daemon *Daemon) GetPodNum() int64 {
	pods, err := daemon.db.ListPod()
	if err != nil {
		return 0
	}
	return int64(len(pods))
}

func (daemon *Daemon) GetRunningPodNum() int64 {
	return daemon.PodList.CountRunning()
}

func (daemon *Daemon) GetVolumeId(podId, volName string) (int, error) {
	vols, err := daemon.db.ListPodVolumes(podId)
	if err != nil {
		return -1, err
	}

	dev_id := 0
	for _, vol := range vols {
		fields := strings.Split(string(vol), ":")
		if fields[0] == volName {
			dev_id, _ = strconv.Atoi(fields[1])
		}
	}
	return dev_id, nil
}

func (daemon *Daemon) DeleteVolumeId(podId string) error {
	vols, err := daemon.db.ListPodVolumes(podId)
	if err != nil {
		return err
	}
	for _, vol := range vols {
		daemon.Storage.RemoveVolume(podId, vol)
	}
	return daemon.db.DeletePodVolumes(podId)
}

func (daemon *Daemon) WritePodAndContainers(podId string) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("Cannot find Pod %s to write", podId)
	}

	containers := []string{}
	for _, c := range p.PodStatus.Containers {
		containers = append(containers, c.Id)
	}

	return daemon.db.UpdateP2C(podId, containers)
}

func (daemon *Daemon) GetVmByPodId(podId string) (string, error) {
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return "", fmt.Errorf("Not found Pod %s", podId)
	}
	return pod.PodStatus.Vm, nil
}

func (daemon *Daemon) GetPodByContainer(containerId string) (string, error) {
	if pod, ok := daemon.PodList.GetByContainerId(containerId); ok {
		return pod.Id, nil
	} else {
		return "", fmt.Errorf("Can not find that container!")
	}
}

func (daemon *Daemon) GetPodByContainerIdOrName(name string) (pod *Pod, idx int, err error) {
	if pod, idx, ok := daemon.PodList.GetByContainerIdOrName(name); ok {
		return pod, idx, nil
	} else {
		return nil, -1, fmt.Errorf("cannot find container %s", name)
	}
}

func (daemon *Daemon) AddPod(pod *Pod, podArgs string) (err error) {
	// store the UserPod into the db
	if err = daemon.db.UpdatePod(pod.Id, []byte(podArgs)); err != nil {
		glog.V(1).Info("Found an error while saving the POD file")
		return
	}
	defer func() {
		if err != nil {
			daemon.db.DeletePod(pod.Id)
		}
	}()

	daemon.PodList.Put(pod)
	defer func() {
		if err != nil {
			daemon.PodList.Delete(pod.Id)
		}
	}()

	if err = daemon.WritePodAndContainers(pod.Id); err != nil {
		glog.V(1).Info("Found an error while saving the Containers info")
		return
	}

	return nil
}

func (daemon *Daemon) RemovePod(podId string) {
	daemon.PodList.Delete(podId)
}

func (daemon *Daemon) AddVm(vm *hypervisor.Vm) {
	daemon.VmList.Add(vm)
}

func (daemon *Daemon) RemoveVm(vmId string) {
	daemon.VmList.Remove(vmId)
}

func (daemon *Daemon) DestroyAllVm() error {
	var remains = []*Pod{}
	daemon.PodList.Foreach(func(p *Pod) error {
		remains = append(remains, p)
		return nil
	})
	for _, p := range remains {
		if _, _, err := daemon.StopPodWithinLock(p); err != nil {
			glog.V(1).Infof("fail to stop %s: %v", p.Id, err)
		}
	}
	return nil
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

	daemon.VmList.Foreach(func(vm *hypervisor.Vm) error {
		if _, _, err := vm.Kill(); err == nil {
			delete(daemon.VmList.vms, vm.Id)
		}
		return nil
	})

	daemon.db.Close()
	glog.Flush()
	return nil
}
