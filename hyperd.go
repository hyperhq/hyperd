package main

import (
	"os"
	"fmt"
	"flag"
	"syscall"
	"os/signal"

	"hyper/engine"
	"hyper/utils"
	"hyper/lib/glog"
	"hyper/hyperdaemon"
)

func main() {
	flConfig := flag.String("config", "", "Config file for hyperd")
	flHost := flag.String("host", "", "Host for hyperd")
	flHelp := flag.Bool("help", false, "Print help message for Hyperd daemon")
	glog.Init()
	flag.Usage = func() {printHelp()}
	flag.Parse()
	if *flHelp == true {
		printHelp()
		return
	}
	mainDaemon(*flConfig, *flHost)
}

func printHelp() {
	var helpMessage = `Usage:
  %s [OPTIONS]

Application Options:
  --config=""            configuration for %s
  --v=0                  log level fro V logs
  --log_dir              log directory
  --host                 host address and port for hyperd(such as --host=tcp://127.0.0.1:12345)
  --logtostderr          log to standard error instead of files
  --alsologtostderr      log to standard error as well as files

Help Options:
  -h, --help             Show this help message

`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
}

func mainDaemon(config, host string) {
	glog.V(0).Infof("The config file is %s", config)
	if config == "" {
		config = "/etc/hyper/config"
	}
	eng := engine.New(config)

	d, err := daemon.NewDaemon(eng)
	if err != nil {
		glog.Error("The hyperd create failed, %s\n", err.Error())
		return
	}

	stopAll := make(chan os.Signal, 1)
	signal.Notify(stopAll, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP)

	// Install the accepted jobs
	if err := d.Install(eng); err != nil {
		glog.Error("The hyperd install failed, %s\n", err.Error())
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
