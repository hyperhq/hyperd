package legacy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hyperhq/hyperd/utils"
)

// Pod Data Structure
type UserUser struct {
	Name             string   `json:"name"`
	Group            string   `json:"group"`
	AdditionalGroups []string `json:"additionalGroups,omitempty"`
}

type UserContainerPort struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	ServicePort   int    `json:"servicePort"`
	Protocol      string `json:"protocol"`
}

type UserEnvironmentVar struct {
	Env   string `json:"env"`
	Value string `json:"value"`
}

type UserVolumeReference struct {
	Path     string `json:"path"`
	Volume   string `json:"volume"`
	ReadOnly bool   `json:"readOnly"`
}

type UserFileReference struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Perm     string `json:"perm"`
	User     string `json:"user"`
	Group    string `json:"group"`
}

type UserContainer struct {
	Name          string                `json:"name"`
	Image         string                `json:"image"`
	User          UserUser              `json:"user,omitempty"`
	Command       []string              `json:"command"`
	Workdir       string                `json:"workdir"`
	Entrypoint    []string              `json:"entrypoint"`
	Tty           bool                  `json:"tty,omitempty"`
	Sysctl        map[string]string     `json:"sysctl,omitempty"`
	Labels        map[string]string     `json:"labels"`
	Ports         []UserContainerPort   `json:"ports"`
	Envs          []UserEnvironmentVar  `json:"envs"`
	Volumes       []UserVolumeReference `json:"volumes"`
	Files         []UserFileReference   `json:"files"`
	RestartPolicy string                `json:"restartPolicy"`
}

type UserResource struct {
	Vcpu   int `json:"vcpu"`
	Memory int `json:"memory"`
}

type UserFile struct {
	Name     string `json:"name"`
	Encoding string `json:"encoding"`
	Uri      string `json:"uri"`
	Contents string `json:"content"`
}

type UserVolumeOption struct {
	Monitors    []string `json:"monitors"`
	User        string   `json:"user"`
	Keyring     string   `json:"keyring"`
	BytesPerSec int      `json:"bytespersec"`
	Iops        int      `json:"iops"`
}

type UserVolume struct {
	Name   string           `json:"name"`
	Source string           `json:"source"`
	Driver string           `json:"driver"`
	Option UserVolumeOption `json:"option,omitempty"`
}

type UserInterface struct {
	Bridge string `json:"bridge"`
	Ip     string `json:"ip"`
	Ifname string `json:"ifname,omitempty"`
	Mac    string `json:"mac,omitempty"`
	Gw     string `json:"gateway,omitempty"`
}

type UserServiceBackend struct {
	HostIP   string `json:"hostip"`
	HostPort int    `json:"hostport"`
}

type UserService struct {
	ServiceIP   string               `json:"serviceip"`
	ServicePort int                  `json:"serviceport"`
	Protocol    string               `json:"protocol"`
	Hosts       []UserServiceBackend `json:"hosts"`
}

type PodLogConfig struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

type PortmappingWhiteList struct {
	InternalNetworks []string `json:"internalNetworks,omitempty"`
	ExternalNetworks []string `json:"externalNetworks,omitempty"`
}

type UserPod struct {
	Name                  string                `json:"id"`
	Hostname              string                `json:"hostname"`
	Containers            []UserContainer       `json:"containers"`
	Resource              UserResource          `json:"resource"`
	Files                 []UserFile            `json:"files"`
	Volumes               []UserVolume          `json:"volumes"`
	Interfaces            []UserInterface       `json:"interfaces,omitempty"`
	Labels                map[string]string     `json:"labels"`
	Services              []UserService         `json:"services,omitempty"`
	LogConfig             PodLogConfig          `json:"log"`
	Dns                   []string              `json:"dns,omitempty"`
	PortmappingWhiteLists *PortmappingWhiteList `json:"portmappingWhiteLists,omitempty"`
	Tty                   bool                  `json:"tty"`
	Type                  string                `json:"type"`
	RestartPolicy         string
}

func ProcessPodFile(jsonFile string) (*UserPod, error) {
	if _, err := os.Stat(jsonFile); err != nil && os.IsNotExist(err) {
		return nil, err
	}
	body, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		return nil, err
	}
	return ProcessPodBytes(body)
}

func ProcessPodBytes(body []byte) (*UserPod, error) {
	var userPod UserPod
	if err := json.Unmarshal(body, &userPod); err != nil {
		return nil, err
	}

	// we need to validate the given POD file
	if userPod.Name == "" {
		userPod.Name = utils.RandStr(10, "alphanum")
	}

	if userPod.Resource.Vcpu == 0 {
		userPod.Resource.Vcpu = 1
	}
	if userPod.Resource.Memory == 0 {
		userPod.Resource.Memory = 128
	}

	var (
		vol UserVolume
		num = 0
	)
	for i, v := range userPod.Containers {
		if v.Image == "" {
			return nil, fmt.Errorf("Please specific your image for your container, it can not be null!\n")
		}
		userPod.Containers[i].Tty = v.Tty || userPod.Tty
		num++
	}
	if num == 0 {
		return nil, fmt.Errorf("Please correct your POD file, the container section can not be null!\n")
	}
	for _, vol = range userPod.Volumes {
		if vol.Name == "" {
			return nil, fmt.Errorf("Hyper ERROR: please specific your volume name, it can not be null!\n")
		}
	}

	if userPod.Labels == nil {
		userPod.Labels = make(map[string]string)
	}

	return &userPod, nil
}
