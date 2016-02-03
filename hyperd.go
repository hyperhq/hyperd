package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Unknwon/goconfig"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/daemon/graphdriver/vbox"
	"github.com/hyperhq/hyper/server"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/hypervisor"

	runvutils "github.com/hyperhq/runv/lib/utils"
	"github.com/kardianos/osext"
)

type Options struct {
	DisableIptables    bool
	Config             string
	Hosts              string
	Mirrors            string
	InsecureRegistries string
}

func main() {
	if reexec.Init() {
		return
	}

	fnd := flag.Bool("nondaemon", false, "Not daemonize")
	flDisableIptables := flag.Bool("noniptables", false, "Don't enable iptables rules")
	flConfig := flag.String("config", "", "Config file for hyperd")
	flHost := flag.String("host", "", "Host for hyperd")
	flMirrors := flag.String("registry_mirror", "", "Prefered docker registry mirror")
	flInsecureRegistries := flag.String("insecure_registry", "", "Enable insecure registry communication")
	flHelp := flag.Bool("help", false, "Print help message for Hyperd daemon")
	flag.Set("alsologtostderr", "true")
	flag.Set("log_dir", "/var/log/hyper/")
	os.MkdirAll("/var/log/hyper/", 0755)
	flag.Usage = func() { printHelp() }
	flag.Parse()
	if *flHelp == true {
		printHelp()
		return
	}

	if !*fnd {
		path, err := osext.Executable()
		if err != nil {
			fmt.Printf("cannot find self executable path for %s: %v\n", os.Args[0], err)
			os.Exit(-1)
		}

		_, err = runvutils.ExecInDaemon(path, append([]string{os.Args[0], "--nondaemon"}, os.Args[1:]...))
		if err != nil {
			fmt.Println("faile to daemonize hyperd")
			os.Exit(-1)
		}

		return
	}

	var opt = &Options{
		DisableIptables:    *flDisableIptables,
		Config:             *flConfig,
		Hosts:              *flHost,
		Mirrors:            *flMirrors,
		InsecureRegistries: *flInsecureRegistries,
	}

	mainDaemon(opt)
}

func printHelp() {
	var helpMessage = `Usage:
  %s [OPTIONS]

Application Options:
  --nondaemon            Not daemonize
  --config=""            Configuration for %s
  --v=0                  Log level fro V logs
  --log_dir              Log directory
  --host                 Host address and port for hyperd(such as --host=tcp://127.0.0.1:12345)
  --registry_mirror      Prefered docker registry mirror, multiple values separated by a comma
  --insecure_registry    Enable insecure registry communication, multiple values separated by a comma
  --logtostderr          Log to standard error instead of files
  --alsologtostderr      Log to standard error as well as files

Help Options:
  -h, --help             Show this help message

`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
}

func mainDaemon(opt *Options) {
	config := opt.Config
	glog.V(1).Infof("The config file is %s", config)
	if config == "" {
		config = "/etc/hyper/config"
	}
	if _, err := os.Stat(config); err != nil {
		if os.IsNotExist(err) {
			glog.Errorf("Can not find config file(%s)", config)
			return
		}
		glog.Errorf(err.Error())
		return
	}

	os.Setenv("HYPER_CONFIG", config)
	cfg, err := goconfig.LoadConfigFile(config)
	if err != nil {
		glog.Errorf("Read config file (%s) failed, %s", config, err.Error())
		return
	}

	hyperRoot, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Root")

	if hyperRoot == "" {
		hyperRoot = "/var/lib/hyper"
	}
	utils.HYPER_ROOT = hyperRoot
	if _, err := os.Stat(hyperRoot); err != nil {
		if err := os.MkdirAll(hyperRoot, 0755); err != nil {
			glog.Errorf(err.Error())
			return
		}
	}

	storageDriver, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "StorageDriver")
	daemon.InitDockerCfg(strings.Split(opt.Mirrors, ","), strings.Split(opt.InsecureRegistries, ","), storageDriver, hyperRoot)
	d, err := daemon.NewDaemon(cfg)
	if err != nil {
		glog.Errorf("The hyperd create failed, %s", err.Error())
		return
	}

	vbox.Register(d)

	serverConfig := &server.Config{}

	defaultHost := "unix:///var/run/hyper.sock"
	Hosts := []string{defaultHost}

	if opt.Hosts != "" {
		Hosts = append(Hosts, opt.Hosts)
	}
	if d.Host != "" {
		Hosts = append(Hosts, d.Host)
	}

	for i := 0; i < len(Hosts); i++ {
		var err error
		if Hosts[i], err = opts.ParseHost(defaultHost, Hosts[i]); err != nil {
			glog.Errorf("error parsing -H %s : %v", Hosts[i], err)
			return
		}

		protoAddr := Hosts[i]
		protoAddrParts := strings.SplitN(protoAddr, "://", 2)
		if len(protoAddrParts) != 2 {
			glog.Errorf("bad format %s, expected PROTO://ADDR", protoAddr)
			return
		}
		serverConfig.Addrs = append(serverConfig.Addrs, server.Addr{Proto: protoAddrParts[0], Addr: protoAddrParts[1]})
	}

	api, err := server.New(serverConfig)
	if err != nil {
		glog.Errorf(err.Error())
		return
	}

	api.InitRouters(d)

	driver, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Hypervisor")
	driver = strings.ToLower(driver)
	if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
		glog.Warningf("%s", err.Error())
		glog.Errorf("Please specify the correct and available hypervisor, such as 'kvm', 'qemu-kvm',  'libvirt', 'xen', 'qemu', 'vbox' or ''")
		return
	} else {
		d.Hypervisor = driver
		glog.Infof("The hypervisor's driver is %s", driver)
	}

	disableIptables := cfg.MustBool(goconfig.DEFAULT_SECTION, "DisableIptables", false)
	if err = hypervisor.InitNetwork(d.BridgeIface, d.BridgeIP, disableIptables || opt.DisableIptables); err != nil {
		glog.Errorf("InitNetwork failed, %s", err.Error())
		return
	}

	defaultLog, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Logger")
	defaultLogCfg, _ := cfg.GetSection("Log")
	d.DefaultLogCfg(defaultLog, defaultLogCfg)

	// Set the daemon object as the global varibal
	// which will be used for puller and builder
	utils.SetDaemon(d)

	// The serve API routine never exits unless an error occurs
	// We need to start it as a goroutine and wait on it so
	// daemon doesn't exit
	serveAPIWait := make(chan error)
	go api.Wait(serveAPIWait)

	stopAll := make(chan os.Signal, 1)
	signal.Notify(stopAll, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP)

	glog.V(0).Infof("Hyper daemon: %s %s",
		utils.VERSION,
		utils.GITCOMMIT,
	)

	if err := d.Restore(); err != nil {
		glog.Warningf("Fail to restore the previous VM")
		return
	}

	vmCachePolicy, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "VmCachePolicy")
	d.InitVmCache(vmCachePolicy)

	// Daemon is fully initialized and handling API traffic
	// Wait for serve API job to complete
	select {
	case errAPI := <-serveAPIWait:
		// If we have an error here it is unique to API (as daemonErr would have
		// exited the daemon process above)
		if errAPI != nil {
			glog.Warningf("Shutting down due to ServeAPI error: %v", errAPI)
		}
		break
	case <-stop:
		d.DestroyAndKeepVm()
		break
	case <-stopAll:
		d.DestroyAllVm()
		break
	}
	api.Close()
	d.Shutdown()
}
