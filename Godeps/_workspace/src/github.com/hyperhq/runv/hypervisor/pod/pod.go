package pod

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/hyperhq/runv/lib/utils"
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
		userPod.Name = RandStr(10, "alphanum")
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

func RandStr(strSize int, randType string) string {
	var dictionary string
	if randType == "alphanum" {
		dictionary = "0123456789abcdefghijklmnopqrstuvwxyz"
	}

	if randType == "alpha" {
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}

	if randType == "number" {
		dictionary = "0123456789"
	}

	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for k, v := range bytes {
		bytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(bytes)
}

//validate
// 1. volume name + mount point is unique
// 2. file name is unique
// 3. container should not use volume/file not in volume/file list
// 4. environment var should be uniq in one container
func (pod *UserPod) Validate() error {
	var volume_drivers = map[string]bool{
		"raw":   true,
		"qcow2": true,
		"vdi":   true,
		"vfs":   true,
		"rbd":   true,
	}

	hostnameLen := len(pod.Hostname)
	if hostnameLen > 63 {
		return fmt.Errorf("Hostname exceeds the maximum length 63, len: %d", hostnameLen)
	}
	if hostnameLen > 0 {
		for _, seg := range strings.Split(pod.Hostname, ".") {
			if !utils.IsDNSLabel(seg) {
				return fmt.Errorf("Hostname should fullfil the pattern: %s, input hostname: %s", utils.Dns1123LabelFmt, pod.Hostname)
			}
		}
	}

	hasGw := false
	for idx, config := range pod.Interfaces {
		if config.Gw == "" {
			continue
		}
		if hasGw {
			return fmt.Errorf("in interface %d, Other interface already configured Gateway", idx)
		}
		hasGw = true
	}

	uniq, vset := keySet(pod.Volumes)
	if !uniq {
		if len(vset) > 0 {
			return errors.New("Volumes name does not unique")
		}
	}

	uniq, fset := keySet(pod.Files)
	if !uniq {
		if len(fset) > 0 {
			return errors.New("Files name does not unique")
		}
	}
	var permReg = regexp.MustCompile("0[0-7]{3}")
	for idx, container := range pod.Containers {

		if uniq, _ := keySet(container.Volumes); !uniq {
			return fmt.Errorf("in container %d, volume source are not unique", idx)
		}

		if uniq, _ := keySet(container.Envs); !uniq {
			return fmt.Errorf("in container %d, environment name are not unique", idx)
		}

		for _, f := range container.Files {
			if _, ok := fset[f.Filename]; !ok {
				return fmt.Errorf("in container %d, file %s does not exist in file list.", idx, f.Filename)
			}
			if f.Perm == "" {
				f.Perm = "0755"
			}
			if f.Perm != "0" {
				if !permReg.Match([]byte(f.Perm)) {
					return fmt.Errorf("in container %d, the permission %s only accept Octal digital in string", idx, f.Perm)
				}
			}
		}

		for _, v := range container.Volumes {
			if _, ok := vset[v.Volume]; !ok {
				return fmt.Errorf("in container %d, volume %s does not exist in volume list.", idx, v.Volume)
			}
		}
	}

	for idx, v := range pod.Volumes {
		if v.Driver == "" {
			continue
		}

		if _, ok := volume_drivers[v.Driver]; !ok {
			return fmt.Errorf("in volume %d, volume does not support driver %s.", idx, v.Driver)
		}
	}

	for _, dns := range pod.Dns {
		if ip := net.ParseIP(dns); ip == nil {
			return fmt.Errorf("incorrect dns %s.", dns)
		}
	}

	return nil
}

type item interface {
	key() string
}

func keySet(ilist interface{}) (bool, map[string]bool) {
	tmp, err := InterfaceSlice(ilist)
	if err != nil {
		return false, nil
	}
	iset := make(map[string]bool)
	for _, x := range tmp {
		switch x.(type) {
		case item:
			kx := x.(item).key()
			if _, ok := iset[kx]; ok {
				return false, iset
			}
			iset[kx] = true
			break
		default:
			return false, iset
		}
	}
	return true, iset
}

func (vol UserVolume) key() string          { return vol.Name }
func (vol UserVolumeReference) key() string { return "vol:" + vol.Volume + "/mnt:" + vol.Path }
func (f UserFile) key() string              { return f.Name }
func (env UserEnvironmentVar) key() string  { return env.Env }

func InterfaceSlice(slice interface{}) ([]interface{}, error) {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		return nil, fmt.Errorf("InterfaceSlice() given a non-slice type")
	}

	ret := make([]interface{}, s.Len())

	for i := 0; i < s.Len(); i++ {
		ret[i] = s.Index(i).Interface()
	}

	return ret, nil
}
