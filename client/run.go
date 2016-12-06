package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/namesgenerator"
	gflag "github.com/jessevdk/go-flags"

	apitype "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/lib/term"
)

// hyperctl run [OPTIONS] image [COMMAND] [ARGS...]
func (cli *HyperClient) HyperCmdRun(args ...string) (err error) {
	var opts struct {
		PodFile       string   `short:"p" long:"podfile" value-name:"\"\"" description:"Create and Run a pod based on the pod file"`
		Yaml          bool     `short:"y" long:"yaml" default:"false" default-mask:"-" description:"Create a pod based on Yaml file"`
		Name          string   `long:"name" value-name:"\"\"" description:"Assign a name to the container"`
		Attach        bool     `short:"a" long:"attach" default:"false" default-mask:"-" description:"(from podfile) Attach the stdin, stdout and stderr to the container"`
		Detach        bool     `short:"d" long:"detach" default:"false" default-mask:"-" description:"(from cmdline) Not Attach the stdin, stdout and stderr to the container"`
		Workdir       string   `long:"workdir" value-name:"\"\"" default-mask:"-" description:"Working directory inside the container"`
		Tty           bool     `short:"t" long:"tty" default:"false" default-mask:"-" description:"the run command in tty, such as bash shell"`
		Cpu           int      `long:"cpu" default:"1" value-name:"1" default-mask:"-" description:"CPU number for the VM"`
		Memory        int      `long:"memory" default:"128" value-name:"128" default-mask:"-" description:"Memory size (MB) for the VM"`
		Env           []string `long:"env" value-name:"[]" default-mask:"-" description:"Set environment variables"`
		EntryPoint    string   `long:"entrypoint" value-name:"\"\"" default-mask:"-" description:"Overwrite the default ENTRYPOINT of the image"`
		RestartPolicy string   `long:"restart" default:"never" value-name:"\"\"" default-mask:"-" description:"Restart policy to apply when a container exits (never, onFailure, always)"`
		LogDriver     string   `long:"log-driver" value-name:"\"\"" description:"Logging driver for Pod"`
		LogOpts       []string `long:"log-opt" description:"Log driver options"`
		Remove        bool     `long:"rm" default:"false" default-mask:"-" description:"Automatically remove the pod when it exits"`
		Portmap       []string `long:"publish" value-name:"[]" default-mask:"-" description:"Publish a container's port to the host, format: --publish [tcp/udp:]hostPort:containerPort"`
		Labels        []string `long:"label" value-name:"[]" default-mask:"-" description:"Add labels for Pod, format: --label key=value"`
		Volumes       []string `short:"v" long:"volume" value-name:"[]" default-mask:"-" description:"Mount host file/directory as a data file/volume, format: -v|--volume=[[hostDir:]containerDir[:options]]"`
	}

	var (
		podId   string
		vmId    string
		podJson string
		attach  bool = false
	)
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "run [OPTIONS] IMAGE [COMMAND] [ARG...]\n\nCreate a pod, and launch a new VM to run the pod"
	args, err = parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if opts.PodFile != "" {
		attach = opts.Attach
		podJson, err = cli.JsonFromFile(opts.PodFile, opts.Yaml, false)
	} else {
		if len(args) == 0 {
			return fmt.Errorf("%s: \"run\" requires a minimum of 1 argument, please provide the image.", os.Args[0])
		}
		attach = !opts.Detach
		podJson, err = cli.JsonFromCmdline(args, opts.Env, opts.Portmap, opts.LogDriver, opts.LogOpts,
			opts.Name, opts.Workdir, opts.RestartPolicy, opts.Cpu, opts.Memory, opts.Tty, opts.Labels, opts.EntryPoint, opts.Volumes)
	}

	if err != nil {
		return err
	}

	t1 := time.Now()

	var (
		spec apitype.UserPod
		code int
		tty  = false
	)
	err = json.Unmarshal([]byte(podJson), &spec)
	if err != nil {
		return err
	}

	podId, code, err = cli.client.CreatePod(&spec)
	if err != nil {
		if code == http.StatusNotFound {
			err = cli.PullImages(&spec)
			if err != nil {
				return
			}
			podId, code, err = cli.client.CreatePod(&spec)
		}
		if err != nil {
			return
		}
	}
	if !attach {
		fmt.Printf("POD id is %s\n", podId)
	}

	if opts.Remove {
		defer func() {
			rmerr := cli.client.RmPod(podId)
			if rmerr != nil {
				fmt.Fprintf(cli.out, "failed to rm pod, %v\n", rmerr)
			}
		}()
	}

	if attach {
		if opts.PodFile == "" {
			tty = opts.Tty
		} else {
			tty = spec.Tty || spec.Containers[0].Tty
		}

		if tty {
			p, err := cli.client.GetPodInfo(podId)
			if err == nil {
				cli.monitorTtySize(p.Spec.Containers[0].ContainerID, "")
			}

			oldState, err := term.SetRawTerminal(cli.inFd)
			if err != nil {
				return err
			}
			defer term.RestoreTerminal(cli.inFd, oldState)
		}

	}

	_, err = cli.client.StartPod(podId, vmId, attach, tty, cli.in, cli.out, cli.err)
	if err != nil {
		return
	}

	if !attach {
		t2 := time.Now()
		fmt.Printf("Time to run a POD is %d ms\n", (t2.UnixNano()-t1.UnixNano())/1000000)
	}
	return nil
}

func (cli *HyperClient) JsonFromFile(filename string, yaml, k8s bool) (string, error) {
	if _, err := os.Stat(filename); err != nil {
		return "", err
	}

	jsonbody, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	if yaml == true {
		jsonbody, err = cli.ConvertYamlToJson(jsonbody)
		if err != nil {
			return "", err
		}
	}

	return string(jsonbody), nil
}

func (cli *HyperClient) JsonFromCmdline(cmdArgs, cmdEnvs, cmdPortmaps []string, cmdLogDriver string, cmdLogOpts []string,
	cmdName, cmdWorkdir, cmdRestartPolicy string, cpu, memory int, tty bool, cmdLabels []string, entrypoint string, cmdVols []string) (string, error) {

	var (
		name       = cmdName
		image      = cmdArgs[0]
		command    = []string{}
		env        = []*apitype.EnvironmentVar{}
		ports      = []*apitype.UserContainerPort{}
		logOpts    = make(map[string]string)
		labels     = make(map[string]string)
		volumesRef = []*apitype.UserVolumeReference{}
		volumes    = []*apitype.UserVolume{}
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
		volumes = append(volumes, vol)
		volumesRef = append(volumesRef, volRef)
	}

	entrypoints := make([]string, 0, 1)
	if len(entrypoint) > 0 {
		entrypoints = append(entrypoints, entrypoint)
	}

	containerList := []*apitype.UserContainer{{
		Name:          name,
		Image:         image,
		Command:       command,
		Workdir:       cmdWorkdir,
		Entrypoint:    entrypoints,
		Ports:         ports,
		Envs:          env,
		Volumes:       volumesRef,
		Files:         []*apitype.UserFileReference{},
		RestartPolicy: cmdRestartPolicy,
		Tty:           tty,
	}}

	userPod := &apitype.UserPod{
		Id:         name,
		Containers: containerList,
		Labels:     labels,
		Resource:   &apitype.UserResource{Vcpu: int32(cpu), Memory: int32(memory)},
		Files:      []*apitype.UserFile{},
		Volumes:    volumes,
		Log: &apitype.PodLogConfig{
			Type:   cmdLogDriver,
			Config: logOpts,
		},
	}

	jsonString, _ := json.Marshal(userPod)
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

func parsePortMapping(portmap string) (*apitype.UserContainerPort, error) {

	var (
		port  = apitype.UserContainerPort{}
		proto string
		hPort string
		cPort string
		err   error
	)

	fields := strings.Split(portmap, ":")
	if len(fields) < 2 {
		return nil, fmt.Errorf("flag needs host port and container port: --publish")
	} else if len(fields) == 2 {
		proto = "tcp"
		hPort = fields[0]
		cPort = fields[1]
	} else {
		proto = fields[0]
		if proto != "tcp" && proto != "udp" {
			return nil, fmt.Errorf("flag needs protocol(tcp or udp): --publish")
		}
		hPort = fields[1]
		cPort = fields[2]
	}

	port.Protocol = proto
	hp, err := strconv.Atoi(hPort)
	port.HostPort = int32(hp)
	if err != nil {
		return nil, fmt.Errorf("flag needs host port and container port: --publish: %v", err)
	}
	cp, err := strconv.Atoi(cPort)
	port.ContainerPort = int32(cp)
	if err != nil {
		return nil, fmt.Errorf("flag needs host port and container port: --publish: %v", err)
	}

	return &port, nil
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
