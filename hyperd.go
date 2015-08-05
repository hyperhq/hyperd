package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/docker"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/docker/pkg/reexec"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/glog"

	"github.com/Unknwon/goconfig"
)

func main() {
	if reexec.Init() {
		return
	}

	flConfig := flag.String("config", "", "Config file for hyperd")
	flHost := flag.String("host", "", "Host for hyperd")
	flHelp := flag.Bool("help", false, "Print help message for Hyperd daemon")
	flDriver := flag.String("driver", "", "Exec driver for Hyperd daemon")
	glog.Init()
	flag.Usage = func() { printHelp() }
	flag.Parse()
	if *flHelp == true {
		printHelp()
		return
	}
	if *flDriver == "" || (*flDriver != "kvm" && *flDriver != "xen" && *flDriver != "vbox") {
		fmt.Printf("Please specify the exec driver, such as 'kvm', 'xen' or 'vbox'\n")
		return
	}
	mainDaemon(*flConfig, *flHost, *flDriver)
}

func printHelp() {
	var helpMessage = `Usage:
  %s [OPTIONS]

Application Options:
  --config=""            Configuration for %s
  --driver=""            Exec driver for hyperd daemon
  --v=0                  Log level fro V logs
  --log_dir              Log directory
  --host                 Host address and port for hyperd(such as --host=tcp://127.0.0.1:12345)
  --logtostderr          Log to standard error instead of files
  --alsologtostderr      Log to standard error as well as files

Help Options:
  -h, --help             Show this help message

`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
}

func mainDaemon(config, host, driver string) {
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
	iso, _ := cfg.GetValue(goconfig.DEFAULT_SECTION, "Vbox")

	if hyperRoot == "" {
		hyperRoot = "/var/lib/hyper"
	}
	if _, err := os.Stat(hyperRoot); err != nil {
		if err := os.MkdirAll(hyperRoot, 0755); err != nil {
			glog.Errorf(err.Error())
			return
		}
	}

	utils.SetHyperEnv(fmt.Sprintf("%s/.hyperconfig", os.Getenv("HOME")), hyperRoot, iso)
	eng := engine.New(config)
	docker.Init()

	d, err := daemon.NewDaemon(eng)
	if err != nil {
		glog.Errorf("The hyperd create failed, %s\n", err.Error())
		return
	}

	if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
		glog.Errorf("%s\n", err.Error())
		return
	}
	// Set the daemon object as the global varibal
	// which will be used for puller and builder
	utils.SetDaemon(d)
	if err := d.DockerCli.Setup(); err != nil {
		glog.Error(err.Error())
		return
	}

	stopAll := make(chan os.Signal, 1)
	signal.Notify(stopAll, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP)

	// Install the accepted jobs
	if err := d.Install(eng); err != nil {
		glog.Errorf("The hyperd install failed, %s\n", err.Error())
		return
	}

	glog.V(0).Infof("Hyper daemon: %s %s\n",
		utils.VERSION,
		utils.GITCOMMIT,
	)

	// after the daemon is done setting up we can tell the api to start
	// accepting connections
	if err := eng.Job("acceptconnections").Run(); err != nil {
		glog.Error("the acceptconnections job run failed!\n")
		return
	}
	defaultHost := []string{}
	if host != "" {
		defaultHost = append(defaultHost, host)
	}
	defaultHost = append(defaultHost, "unix:///var/run/hyper.sock")
	if d.Host != "" {
		defaultHost = append(defaultHost, d.Host)
	}

	job := eng.Job("serveapi", defaultHost...)

	// The serve API job never exits unless an error occurs
	// We need to start it as a goroutine and wait on it so
	// daemon doesn't exit
	serveAPIWait := make(chan error)
	go func() {
		if err := job.Run(); err != nil {
			glog.Errorf("ServeAPI error: %v\n", err)
			serveAPIWait <- err
			return
		}
		serveAPIWait <- nil
	}()

	glog.V(0).Info("Daemon has completed initialization\n")

	if err := d.Restore(); err != nil {
		glog.Warningf("Fail to restore the previous VM")
		return
	}

	// Daemon is fully initialized and handling API traffic
	// Wait for serve API job to complete
	select {
	case errAPI := <-serveAPIWait:
		// If we have an error here it is unique to API (as daemonErr would have
		// exited the daemon process above)
		eng.Shutdown()
		if errAPI != nil {
			glog.Warningf("Shutting down due to ServeAPI error: %v\n", errAPI)
		}
		break
	case <-stop:
		d.DestroyAndKeepVm()
		eng.Shutdown()
		break
	case <-stopAll:
		d.DestroyAllVm()
		eng.Shutdown()
		break
	}
}
