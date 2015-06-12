package docker

import (
	"encoding/json"
	"hyper/lib/glog"
)

type TypeConfig struct {
	AttachStderr    bool                `json:"AttachStderr"`
	AttachStdin     bool                `json:"AttachStdin"`
	AttachStdout    bool                `json:"AttachStdout"`
	Cmd             []string            `json:"Cmd"`
	CpuShares       int64               `json:"CpuShares"`
	Cpuset          string              `json:"Cpuset"`
	Domainname      string              `json:"Domainname"`
	Entrypoint      []string            `json:"Entrypoint"`
	Env             []string            `json:"Env"`
	ExposedPorts    map[string]struct{} `json:"ExposedPorts"`
	Hostname        string              `json:"Hostname"`
	Image           string              `json:"Image"`
	MacAddress      string              `json:"MacAddress"`
	Memory          int64               `json:"Memory"`
	MemorySwap      int64               `json:"MemorySwap"`
	NetworkDisabled bool                `json:"NetworkDisabled"`
	OnBuild         []string            `json:"OnBuild"`
	OpenStdin       bool                `json:"OpenStdin"`
	PortSpecs       []string            `json:"PortSpecs"`
	StdinOnce       bool                `json:"StdinOnce"`
	Tty             bool                `json:"Tty"`
	User            string              `json:"User"`
	Volumes         map[string]struct{} `json:"Volumes"`
	WorkingDir      string              `json:"WorkingDir"`
}

type ConfigJSON struct {
	AppArmorProfile string     `json:"AppArmorProfile"`
	Args            []string   `json:"Args"`
	Config          TypeConfig `json:"Config"`
	Created         string     `json:"Created"`
	Driver          string     `json:"Driver"`
	ExecDriver      string     `json:"ExecDriver"`
	ExecIds         string     `json:"ExecIDs"`
	HostConfig      struct{}   `json:"HostConfig"`
	HostnamePath    string     `json:"HostnamePath"`
	HostsPath       string     `json:"HostsPath"`
	Id              string     `json:"Id"`
	Image           string     `json:"Image"`
	MountLabel      string     `json:"MountLabel"`
	Name            string     `json:"Name"`
	NetworkSettings struct{}   `json:"NetworkSettings"`
	Path            string     `json:"Path"`
	ProcessLabel    string     `json:"ProcessLabel"`
	ResolvConfPath  string     `json:"ResolvConfPath"`
	RestartCount    int        `json:"RestartCount"`
	State           struct{}   `json:"State"`
	Volumes         struct{}   `json:"Volumes"`
	VolumesRW       struct{}   `json:"VolumesRW"`
}

func (cli *DockerCli) GetContainerInfo(args ...string) (*ConfigJSON, error) {
	containerId := args[0]
	glog.V(1).Infof("ready to get the container(%s) info\n", containerId)
	body, _, err := readBody(cli.Call("GET", "/containers/"+containerId+"/json", nil, nil))
	if err != nil {
		return nil, err
	}
	var jsonResponse ConfigJSON
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, err
	}

	return &jsonResponse, nil
}
