package pod

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

// Pod Data Structure
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
	Command       []string              `json:"command"`
	Workdir       string                `json:"workdir"`
	Entrypoint    []string              `json:"entrypoint"`
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

type UserVolume struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Driver string `json:"driver"`
}

type UserPod struct {
	Name       string          `json:"id"`
	Containers []UserContainer `json:"containers"`
	Resource   UserResource    `json:"resource"`
	Files      []UserFile      `json:"files"`
	Volumes    []UserVolume    `json:"volumes"`
	Tty        bool            `json:"tty"`
	Type       string          `json:"type"`
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
		v   UserContainer
		vol UserVolume
		num = 0
	)
	for _, v = range userPod.Containers {
		if v.Image == "" {
			return nil, fmt.Errorf("Please specific your image for your container, it can not be null!\n")
		}
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
// 1. volume name, file name is unique
// 2. source mount to only one pos in one container
// 3. container should not use volume/file not in volume/file list
// 4. environment var should be uniq in one container
func (pod *UserPod) Validate() error {
	uniq, vset := keySet(pod.Volumes)
	if !uniq {
		return errors.New("Volumes name does not unique")
	}

	uniq, fset := keySet(pod.Files)
	if !uniq {
		return errors.New("Files name does not unique")
	}

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
		}

		for _, v := range container.Volumes {
			if _, ok := vset[v.Volume]; !ok {
				return fmt.Errorf("in container %d, volume %s does not exist in file list.", idx, v.Volume)
			}
		}
	}

	return nil
}

type item interface {
	key() string
}

func keySet(ilist interface{}) (bool, map[string]bool) {
	iset := make(map[string]bool)
	switch ilist.(type) {
	case []interface{}:
		for _, x := range ilist.([]interface{}) {
			switch x.(type) {
			case item:
				kx := x.(item).key()
				if _, ok := iset[kx]; ok {
					return false, iset
				}
				iset[kx] = true
			default:
				return false, iset
			}
		}
		return true, iset
	default:
		return false, iset
	}
}

func (vol UserVolume) key() string          { return vol.Name }
func (vol UserVolumeReference) key() string { return vol.Volume }
func (f UserFile) key() string              { return f.Name }
func (env UserEnvironmentVar) key() string  { return env.Env }
