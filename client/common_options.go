package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/docker/docker/pkg/namesgenerator"

	apitype "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
)

type CommonFlags struct {
	PodFile       string   `short:"p" long:"podfile" value-name:"\"\"" description:"read spec from the pod file instead of command line"`
	Yaml          bool     `short:"y" long:"yaml" default-mask:"-" description:"pod file in Yaml format instead of JSON"`
	Name          string   `long:"name" value-name:"\"\"" description:"Assign a name to the container"`
	Workdir       string   `long:"workdir" value-name:"\"\"" default-mask:"-" description:"Working directory inside the container"`
	Tty           bool     `short:"t" long:"tty" default-mask:"-" description:"the run command in tty, such as bash shell"`
	ReadOnly      bool     `long:"read-only" default-mast:"-" description:"Create container with read-only rootfs"`
	Cpu           int      `long:"cpu" default:"1" value-name:"1" default-mask:"-" description:"CPU number for the VM"`
	Memory        int      `long:"memory" default:"128" value-name:"128" default-mask:"-" description:"Memory size (MB) for the VM"`
	Env           []string `long:"env" value-name:"[]" default-mask:"-" description:"Set environment variables"`
	EntryPoint    string   `long:"entrypoint" value-name:"\"\"" default-mask:"-" description:"Overwrite the default ENTRYPOINT of the image"`
	RestartPolicy string   `long:"restart" default:"never" value-name:"\"\"" default-mask:"-" description:"Restart policy to apply when a container exits (never, onFailure, always)"`
	LogDriver     string   `long:"log-driver" value-name:"\"\"" description:"Logging driver for Pod"`
	LogOpts       []string `long:"log-opt" description:"Log driver options"`
	Portmap       []string `long:"publish" value-name:"[]" default-mask:"-" description:"Publish a container's port to the host, format: --publish [tcp/udp:]hostPort:containerPort"`
	Labels        []string `long:"label" value-name:"[]" default-mask:"-" description:"Add labels for Pod, format: --label key=value"`
	Volumes       []string `short:"v" long:"volume" value-name:"[]" default-mask:"-" description:"Mount host file/directory as a data file/volume, format: -v|--volume=[[hostDir:]containerDir[:options]]"`
}

type CreateFlags struct {
	CommonFlags
	Container bool `short:"c" long:"container" default-mast:"-" description:"Create container inside a pod"`
}

type RunFlags struct {
	CommonFlags
	Attach bool `short:"a" long:"attach" default-mask:"-" description:"(from podfile) Attach the stdin, stdout and stderr to the container"`
	Detach bool `short:"d" long:"detach" default-mask:"-" description:"(from cmdline) Not Attach the stdin, stdout and stderr to the container"`
	Remove bool `long:"rm" default-mask:"-" description:"Automatically remove the pod when it exits"`
}

func (cli *HyperClient) ParseCommonOptions(opts *CommonFlags, container bool, args ...string) ([]byte, error) {
	var (
		specJson string
		err      error
	)

	if opts.PodFile != "" {
		specJson, err = cli.JsonFromFile(opts.PodFile, container, opts.Yaml, false)
	} else {
		if len(args) == 0 {
			return nil, fmt.Errorf("%s: this command requires a minimum of 1 argument, please provide the image.", os.Args[0])
		}
		specJson, err = cli.JsonFromCmdline(container, args, opts.Env, opts.Portmap, opts.LogDriver, opts.LogOpts,
			opts.Name, opts.Workdir, opts.RestartPolicy, opts.Cpu, opts.Memory, opts.Tty, opts.ReadOnly, opts.Labels, opts.EntryPoint, opts.Volumes)
	}

	if err != nil {
		return nil, err
	}

	return []byte(specJson), nil
}

func (cli *HyperClient) JsonFromFile(filename string, container, yaml, k8s bool) (string, error) {
	if _, err := os.Stat(filename); err != nil {
		return "", err
	}

	jsonbody, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	if yaml == true {
		jsonbody, err = cli.ConvertYamlToJson(jsonbody, container)
		if err != nil {
			return "", err
		}
	}

	return string(jsonbody), nil
}

func (cli *HyperClient) JsonFromCmdline(container bool, cmdArgs, cmdEnvs, cmdPortmaps []string, cmdLogDriver string, cmdLogOpts []string,
	cmdName, cmdWorkdir, cmdRestartPolicy string, cpu, memory int, tty, readonly bool, cmdLabels []string, entrypoint string, cmdVols []string) (string, error) {

	var (
		name       = cmdName
		image      = cmdArgs[0]
		command    = []string{}
		env        = []*apitype.EnvironmentVar{}
		ports      = []*apitype.PortMapping{}
		logOpts    = make(map[string]string)
		labels     = make(map[string]string)
		volumesRef = []*apitype.UserVolumeReference{}
	)
	if len(cmdArgs) > 1 {
		command = cmdArgs[1:]
	}
	if name == "" {
		name = imageToName(image)
	}
	if memory == 0 {
		memory = 128
	}
	if cpu == 0 {
		cpu = 1
	}
	for _, v := range cmdEnvs {
		if eqlIndex := strings.Index(v, "="); eqlIndex > 0 {
			env = append(env, &apitype.EnvironmentVar{
				Env:   v[:eqlIndex],
				Value: v[eqlIndex+1:],
			})
		}
	}

	for _, v := range cmdLogOpts {
		eql := strings.Index(v, "=")
		if eql > 0 {
			logOpts[v[:eql]] = v[eql+1:]
		} else {
			logOpts[v] = ""
		}
	}

	for _, v := range cmdPortmaps {
		p, err := parsePortMapping(v)
		if err != nil {
			return "", err
		}
		ports = append(ports, p)
	}

	for _, v := range cmdLabels {
		label := strings.Split(v, "=")
		if len(label) == 2 {
			labels[label[0]] = label[1]
		} else {
			return "", fmt.Errorf("Label '%s' is not in 'k=v' format", v)
		}
	}

	for _, v := range cmdVols {
		vol, volRef, err := parseVolume(v)
		if err != nil {
			return "", err
		}
		volRef.Detail = vol
		volumesRef = append(volumesRef, volRef)
	}

	entrypoints := make([]string, 0, 1)
	if len(entrypoint) > 0 {
		entrypoints = append(entrypoints, entrypoint)
	}

	c := &apitype.UserContainer{
		Name:          name,
		Image:         image,
		Command:       command,
		Workdir:       cmdWorkdir,
		Entrypoint:    entrypoints,
		Envs:          env,
		Volumes:       volumesRef,
		Files:         []*apitype.UserFileReference{},
		RestartPolicy: cmdRestartPolicy,
		Tty:           tty,
		ReadOnly:      readonly,
	}

	var body interface{} = c
	if !container {
		userPod := &apitype.UserPod{
			Id:         name,
			Containers: []*apitype.UserContainer{c},
			Labels:     labels,
			Resource:   &apitype.UserResource{Vcpu: int32(cpu), Memory: int32(memory)},
			Log: &apitype.PodLogConfig{
				Type:   cmdLogDriver,
				Config: logOpts,
			},
			Portmappings: ports,
		}
		body = userPod
	}

	jsonString, _ := json.Marshal(body)
	return string(jsonString), nil
}

func parseVolume(volStr string) (*apitype.UserVolume, *apitype.UserVolumeReference, error) {

	var (
		srcName   string
		destPath  string
		volName   string
		readOnly  = false
		volDriver = "vfs"
	)

	fields := strings.Split(volStr, ":")
	if len(fields) == 3 {
		// cmd: -v host-src:container-dest:rw
		srcName = fields[0]
		destPath = fields[1]
		if fields[2] != "ro" && fields[2] != "rw" {
			return nil, nil, fmt.Errorf("flag only support(ro or rw): --volume")
		}
		if fields[2] == "ro" {
			readOnly = true
		}
	} else if len(fields) == 2 {
		// cmd: -v host-src:container-dest
		srcName = fields[0]
		destPath = fields[1]
	} else if len(fields) == 1 {
		// -v container-dest
		destPath = fields[0]
	} else {
		return nil, nil, fmt.Errorf("flag format should be like : --volume=[host-src:]container-dest[:rw|ro]")
	}

	if !strings.HasPrefix(destPath, "/") {
		return nil, nil, fmt.Errorf("The container-dir must always be an absolute path")
	}

	if srcName == "" {
		// Set default volume driver and use destPath as volume Name
		volDriver = ""
		_, volName = filepath.Split(destPath)
	} else {
		srcName, _ = filepath.Abs(srcName)
		_, volName = filepath.Split(srcName)
		// Auto create the source folder on the host , otherwise hyperd will complain
		if _, err := os.Stat(srcName); err != nil && os.IsNotExist(err) {
			if err := os.MkdirAll(srcName, os.FileMode(0777)); err != nil {
				return nil, nil, err
			}
		}
	}

	vol := apitype.UserVolume{
		// Avoid name collision
		Name:   volName + utils.RandStr(5, "number"),
		Source: srcName,
		Format: volDriver,
	}

	volRef := apitype.UserVolumeReference{
		Volume:   vol.Name,
		Path:     destPath,
		ReadOnly: readOnly,
	}

	return &vol, &volRef, nil
}

func parsePortMapping(portmap string) (*apitype.PortMapping, error) {

	var (
		tmp  *apitype.PortMapping
		port *apitype.PortMapping
		err  error
	)

	fields := strings.Split(portmap, ":")
	if len(fields) < 2 {
		return nil, fmt.Errorf("flag needs host port and container port: --publish")
	} else if len(fields) == 2 {
		tmp = &apitype.PortMapping{
			Protocol:      "tcp",
			ContainerPort: fields[1],
			HostPort:      fields[0],
		}
	} else {
		tmp = &apitype.PortMapping{
			Protocol:      fields[0],
			ContainerPort: fields[2],
			HostPort:      fields[1],
		}
	}

	port, err = tmp.Formalize()
	if err != nil {
		return nil, err
	}
	return port, nil
}

func imageToName(image string) string {
	name := image
	fields := strings.Split(image, "/")
	if len(fields) > 1 {
		name = fields[len(fields)-1]
	}
	fields = strings.Split(name, ":")
	if len(fields) < 2 {
		name = name + "-" + utils.RandStr(10, "number")
	} else {
		name = fields[0] + "-" + fields[1] + "-" + utils.RandStr(10, "number")
	}

	validContainerNameChars := `[a-zA-Z0-9][a-zA-Z0-9_.-]`
	validContainerNamePattern := regexp.MustCompile(`^/?` + validContainerNameChars + `+$`)
	if !validContainerNamePattern.MatchString(name) {
		name = namesgenerator.GetRandomName(0)
	}
	return name
}
