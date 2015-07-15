package docker

import (
	"fmt"
	"github.com/hyperhq/runv/lib/glog"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Hostname        string
	Domainname      string
	User            string
	Memory          int64  // FIXME: we keep it for backward compatibility, it has been moved to hostConfig.
	MemorySwap      int64  // FIXME: it has been moved to hostConfig.
	CpuShares       int64  // FIXME: it has been moved to hostConfig.
	Cpuset          string // FIXME: it has been moved to hostConfig and renamed to CpusetCpus.
	AttachStdin     bool
	AttachStdout    bool
	AttachStderr    bool
	PortSpecs       []string // Deprecated - Can be in the format of 8080/tcp
	Tty             bool     // Attach standard streams to a tty, including stdin if it is not closed.
	OpenStdin       bool     // Open stdin
	StdinOnce       bool     // If true, close stdin after the 1 attached client disconnects.
	Env             []string
	Cmd             []string
	Image           string // Name of the image as it was passed by the operator (eg. could be symbolic)
	Volumes         map[string]struct{}
	WorkingDir      string
	Entrypoint      []string
	NetworkDisabled bool
	MacAddress      string
	OnBuild         []string
	SecurityOpt     []string
	Labels          map[string]string
}

type DeviceMapping struct {
	PathOnHost        string
	PathInContainer   string
	CgroupPermissions string
}

type RestartPolicy struct {
	Name              string
	MaximumRetryCount int
}

type LogConfig struct {
	Type   string
	Config map[string]string
}

type HostConfig struct {
	Binds           []string
	ContainerIDFile string
	Memory          int64  // Memory limit (in bytes)
	MemorySwap      int64  // Total memory usage (memory + swap); set `-1` to disable swap
	CpuShares       int64  // CPU shares (relative weight vs. other containers)
	CpusetCpus      string // CpusetCpus 0-2, 0,1
	Privileged      bool
	Links           []string
	PublishAllPorts bool
	Dns             []string
	DnsSearch       []string
	ExtraHosts      []string
	VolumesFrom     []string
	Devices         []DeviceMapping
	NetworkMode     string
	IpcMode         string
	PidMode         string
	CapAdd          []string
	CapDrop         []string
	RestartPolicy   RestartPolicy
	SecurityOpt     []string
	ReadonlyRootfs  bool
	LogConfig       LogConfig
	CgroupParent    string // Parent cgroup.
}

type ConfigAndHostConfig struct {
	Config
	HostConfig HostConfig
}

func (cli *DockerCli) SendCmdCreate(args ...string) ([]byte, int, error) {
	// We need to create a container via an image object.  If the image
	// is not stored locally, so we need to pull the image from the Docker HUB.
	// After that, we have prepared the whole stuffs to create a container.

	// Get a Repository name and tag name from the argument, but be careful
	// with the Repository name with a port number.  For example:
	//      localdomain:5000/samba/hipache:latest
	image := args[0]
	repos, tag := parseTheGivenImageName(image)
	if tag == "" {
		tag = "latest"
	}

	// Pull the image from the docker HUB
	v := url.Values{}
	v.Set("fromImage", repos)
	v.Set("tag", tag)
	imageAndTag := fmt.Sprintf("%s:%s", repos, tag)
	containerValues := url.Values{}
	config := initAndMergeConfigs(imageAndTag)
	glog.V(1).Infof("The Repository is %s, and the tag is %s\n", repos, tag)
	body, statusCode, err := cli.Call("POST", "/containers/create?"+containerValues.Encode(), config, nil)
	glog.V(1).Infof("The returned status code is %d!\n", statusCode)
	if statusCode == 404 || (err != nil && strings.Contains(err.Error(), repos)) {
		glog.V(1).Infof("can not find the image %s\n", repos)
		glog.V(1).Info("pull the image from the repository!\n")
		err = cli.Stream("POST", "/images/create?"+v.Encode(), nil, os.Stdout, nil)
		if err != nil {
			return nil, -1, err
		}
		body, statusCode, err = cli.Call("POST", "/containers/create?"+containerValues.Encode(), config, nil)
		if err != nil {
			return nil, -1, err
		}
		if statusCode != 201 {
			return nil, statusCode, fmt.Errorf("Container create process encountered error, the status code is %s\n", statusCode)
		}
	}
	//response, err := cli.createContainer(config, hostConfig, hostConfig.ContainerIDFile, *flName)
	return readBody(body, statusCode, err)
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

func initAndMergeConfigs(args ...string) *ConfigAndHostConfig {
	imgName := args[0]
	config := &Config{
		Image: imgName,
	}

	hostConfig := &HostConfig{}

	return &ConfigAndHostConfig{
		*config,
		*hostConfig,
	}
}
