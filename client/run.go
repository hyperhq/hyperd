package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/term"

	gflag "github.com/jessevdk/go-flags"
	"net/http"
)

// hyperctl run [OPTIONS] image [COMMAND] [ARGS...]
func (cli *HyperClient) HyperCmdRun(args ...string) (err error) {
	var opts struct {
		PodFile       string   `short:"p" long:"podfile" value-name:"\"\"" description:"Create and Run a pod based on the pod file"`
		K8s           string   `short:"k" long:"kubernetes" value-name:"\"\"" description:"Create and Run a pod based on the kubernetes pod file"`
		Yaml          bool     `short:"y" long:"yaml" default:"false" default-mask:"-" description:"Create a pod based on Yaml file"`
		Name          string   `long:"name" value-name:"\"\"" description:"Assign a name to the container"`
		Attach        bool     `short:"a" long:"attach" default:"false" default-mask:"-" description:"(from podfile) Attach the stdin, stdout and stderr to the container"`
		Detach        bool     `short:"d" long:"detach" default:"false" default-mask:"-" description:"(from cmdline) Not Attach the stdin, stdout and stderr to the container"`
		Workdir       string   `long:"workdir" default:"/" value-name:"\"\"" default-mask:"-" description:"Working directory inside the container"`
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
	} else if opts.K8s != "" {
		attach = opts.Attach
		podJson, err = cli.JsonFromFile(opts.K8s, opts.Yaml, true)
	} else {
		if len(args) == 0 {
			return fmt.Errorf("%s: \"run\" requires a minimum of 1 argument, please provide the image.", os.Args[0])
		}
		attach = !opts.Detach
		podJson, err = cli.JsonFromCmdline(args, opts.Env, opts.Portmap, opts.LogDriver, opts.LogOpts,
			opts.Name, opts.Workdir, opts.RestartPolicy, opts.Cpu, opts.Memory, opts.Tty, opts.Labels, opts.EntryPoint)
	}

	if err != nil {
		return err
	}

	t1 := time.Now()

	var (
		spec  pod.UserPod
		code  int
		async = true
		tty   = false
	)
	json.Unmarshal([]byte(podJson), &spec)

	vmId, err = cli.client.CreateVm(spec.Resource.Vcpu, spec.Resource.Memory, async)
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			cli.client.RmVm(vmId)
		}
	}()

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
		if opts.PodFile == "" && opts.K8s == "" {
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

	if k8s {
		var kpod pod.KPod

		if err := json.Unmarshal(jsonbody, &kpod); err != nil {
			return "", err
		}
		userpod, err := kpod.Convert()
		if err != nil {
			return "", err
		}
		jsonbody, err = json.Marshal(*userpod)
		if err != nil {
			return "", err
		}
	}

	return string(jsonbody), nil
}

func (cli *HyperClient) JsonFromCmdline(cmdArgs, cmdEnvs, cmdPortmaps []string, cmdLogDriver string, cmdLogOpts []string,
	cmdName, cmdWorkdir, cmdRestartPolicy string, cpu, memory int, tty bool, cmdLabels []string, entrypoint string) (string, error) {

	var (
		name    = cmdName
		image   = cmdArgs[0]
		command = []string{}
		env     = []pod.UserEnvironmentVar{}
		ports   = []pod.UserContainerPort{}
		logOpts = make(map[string]string)
		labels  = make(map[string]string)
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
			env = append(env, pod.UserEnvironmentVar{
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
		ports = append(ports, *p)
	}

	for _, v := range cmdLabels {
		label := strings.Split(v, "=")
		if len(label) == 2 {
			labels[label[0]] = label[1]
		} else {
			return "", fmt.Errorf("Label '%s' is not in 'k=v' format", v)
		}
	}

	entrypoints := make([]string, 0, 1)
	if len(entrypoint) > 0 {
		entrypoints = append(entrypoints, entrypoint)
	}

	containerList := []pod.UserContainer{{
		Name:          name,
		Image:         image,
		Command:       command,
		Workdir:       cmdWorkdir,
		Entrypoint:    entrypoints,
		Ports:         ports,
		Envs:          env,
		Volumes:       []pod.UserVolumeReference{},
		Files:         []pod.UserFileReference{},
		RestartPolicy: cmdRestartPolicy,
	}}

	userPod := &pod.UserPod{
		Name:       name,
		Containers: containerList,
		Labels:     labels,
		Resource:   pod.UserResource{Vcpu: cpu, Memory: memory},
		Files:      []pod.UserFile{},
		Volumes:    []pod.UserVolume{},
		LogConfig: pod.PodLogConfig{
			Type:   cmdLogDriver,
			Config: logOpts,
		},
		Tty: tty,
	}

	jsonString, _ := json.Marshal(userPod)
	return string(jsonString), nil
}

func parsePortMapping(portmap string) (*pod.UserContainerPort, error) {

	var (
		port  = pod.UserContainerPort{}
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
	port.HostPort, err = strconv.Atoi(hPort)
	if err != nil {
		return nil, fmt.Errorf("flag needs host port and container port: --publish: %v", err)
	}
	port.ContainerPort, err = strconv.Atoi(cPort)
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
