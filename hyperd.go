package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/server"
	"github.com/hyperhq/hyperd/serverrpc"
	"github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"

	"github.com/docker/docker/pkg/parsers/kernel"
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

	if os.Geteuid() != 0 {
		glog.Errorf("The Hyper daemon needs to be run as root")
		return
	}

	// hyper needs Linux kernel 3.8.0+
	if err := checkKernel(3, 8, 0); err != nil {
		glog.Errorf(err.Error())
		return
	}

	fnd := flag.Bool("nondaemon", false, "[deprecated flag]") // TODO: remove it when 0.8 is released
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

	if *fnd {
		fmt.Printf("flag --nondaemon is deprecated\n")
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
  --config=""            Configuration for %s
  --v=0                  Log level for V logs
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
	c := types.NewHyperConfig(opt.Config)
	if c == nil {
		return
	}
	c.DisableIptables = c.DisableIptables || opt.DisableIptables

	c.AdvertiseEnv()
	if _, err := os.Stat(c.Root); err != nil {
		if err := os.MkdirAll(c.Root, 0755); err != nil {
			glog.Errorf(err.Error())
			return
		}
	}

	daemon.InitDockerCfg(strings.Split(opt.Mirrors, ","), strings.Split(opt.InsecureRegistries, ","), c.StorageDriver, c.Root)
	d, err := daemon.NewDaemon(c)
	if err != nil {
		glog.Errorf("The hyperd create failed, %s", err.Error())
		return
	}

	// Set the daemon object as the global varibal
	// which will be used for puller and builder
	utils.SetDaemon(d)

	if err := d.Restore(); err != nil {
		glog.Warningf("Fail to restore the previous VM")
		return
	}

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

	if c.GRPCHost != "" {
		rpcServer := serverrpc.NewServerRPC(d)
		defer rpcServer.Stop()

		go func() {
			err := rpcServer.Serve(c.GRPCHost)
			if err != nil {
				glog.Fatalf("Hyper serve RPC error: %v", err)
			}
		}()
	}

	// The serve API routine never exits unless an error occurs
	// We need to start it as a goroutine and wait on it so
	// daemon doesn't exit
	serveAPIWait := make(chan error)
	go api.Wait(serveAPIWait)

	stopAll := make(chan os.Signal, 1)
	signal.Notify(stopAll, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP)

	glog.V(0).Infof("Hyper daemon: %s %s", utils.VERSION, utils.GITCOMMIT)

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

func checkKernel(k, major, minor int) error {
	leastVersionInfo := kernel.VersionInfo{
		Kernel: k,
		Major:  major,
		Minor:  minor,
	}

	if v, err := kernel.GetKernelVersion(); err != nil {
		return err
	} else {
		if kernel.CompareKernelVersion(*v, leastVersionInfo) < 0 {
			msg := fmt.Sprintf("Your Linux kernel(%d.%d.%d) is too old to support Hyper daemon(%d.%d.%d+)",
				v.Kernel, v.Major, v.Minor, k, major, minor)
			return fmt.Errorf(msg)
		}
		return nil
	}
}
