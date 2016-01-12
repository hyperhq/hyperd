package docker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/homedir"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/registry"
	"github.com/golang/glog"
	hyperd "github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/utils"
)

var (
	daemonCfg   = &daemon.Config{}
	registryCfg = &registry.Options{}
)

const (
	defaultTrustKeyFile = "key.json"
	defaultCaFile       = "ca.pem"
	defaultKeyFile      = "key.pem"
	defaultCertFile     = "cert.pem"
)

func presentInHelp(usage string) string { return usage }
func absentFromHelp(string) string      { return "" }

type Docker struct {
	daemon *daemon.Daemon
}

func Init() {
	if daemonCfg.LogConfig.Config == nil {
		daemonCfg.LogConfig.Config = make(map[string]string)
	}
	var errhandler flag.ErrorHandling = flag.ContinueOnError
	flags := flag.NewFlagSet("", errhandler)
	daemonCfg.InstallFlags(flags, presentInHelp)
	registryCfg.InstallFlags(flags, absentFromHelp)
	hyperd.NewDockerImpl = func() (docker hyperd.DockerInterface, e error) {
		docker, e = NewDocker()
		if e != nil {
			return nil, fmt.Errorf("failed to create docker instance")
		}
		return docker, nil
	}
	glog.Info("success to create docker")
}

func migrateKey() (err error) {
	// Migrate trust key if exists at ~/.docker/key.json and owned by current user
	oldPath := filepath.Join(homedir.Get(), ".docker", defaultTrustKeyFile)
	newPath := filepath.Join(getDaemonConfDir(), defaultTrustKeyFile)
	if _, statErr := os.Stat(newPath); os.IsNotExist(statErr) && currentUserIsOwner(oldPath) {
		defer func() {
			// Ensure old path is removed if no error occurred
			if err == nil {
				err = os.Remove(oldPath)
			} else {
				glog.Warningf("Key migration failed, key file not removed at %s", oldPath)
			}
		}()

		if err := os.MkdirAll(getDaemonConfDir(), os.FileMode(0644)); err != nil {
			return fmt.Errorf("Unable to create daemon configuration directory: %s", err)
		}

		newFile, err := os.OpenFile(newPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("error creating key file %q: %s", newPath, err)
		}
		defer newFile.Close()

		oldFile, err := os.Open(oldPath)
		if err != nil {
			return fmt.Errorf("error opening key file %q: %s", oldPath, err)
		}
		defer oldFile.Close()

		if _, err := io.Copy(newFile, oldFile); err != nil {
			return fmt.Errorf("error copying key: %s", err)
		}

		glog.Infof("Migrated key from %s to %s", oldPath, newPath)
	}

	return nil
}

func NewDocker() (*Docker, error) {
	registryService := registry.NewService(registryCfg)
	daemonCfg.TrustKeyPath = getDaemonConfDir() + "/" + defaultTrustKeyFile
	daemonCfg.Root = utils.HYPER_ROOT
	d, err := daemon.NewDaemon(daemonCfg, registryService)
	if err != nil {
		glog.Errorf("Error starting daemon: %v", err)
		return nil, err
	}

	glog.Info("Daemon has completed initialization")
	return &Docker{
		daemon: d,
	}, nil
}

func getDaemonConfDir() string {
	return "/etc/hyper"
}

func currentUserIsOwner(f string) bool {
	if fileInfo, err := system.Stat(f); err == nil && fileInfo != nil {
		if int(fileInfo.UID()) == os.Getuid() {
			return true
		}
	}
	return false
}

func parseTheGivenImageName(image string) (string, string) {
	n := strings.Index(image, "@")
	if n > 0 {
		parts := strings.Split(image, "@")
		return parts[0], parts[1]
	}

	n = strings.LastIndex(image, ":")
	if n < 0 {
		return image, ""
	}
	if tag := image[n+1:]; !strings.Contains(tag, "/") {
		return image[:n], tag
	}
	return image, ""
}

func (cli Docker) Shutdown() error {
	return cli.daemon.Shutdown()
}

func (cli Docker) Setup() error {
	return cli.daemon.Setup()
}
