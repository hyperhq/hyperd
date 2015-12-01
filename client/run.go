package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/docker/pkg/namesgenerator"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/pod"

	gflag "github.com/jessevdk/go-flags"
)

// hyper run [OPTIONS] image [COMMAND] [ARGS...]
func (cli *HyperClient) HyperCmdRun(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s ERROR: Can not accept the 'run' command without argument!\n", os.Args[0])
	}
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
		Remove        bool     `long:"rm" default:"false" value-name:"" default-mask:"-" description:"Automatically remove the pod when it exits"`
		Portmap       []string `long:"publish" value-name:"[]" default-mask:"-" description:"Publish a container's port to the host, format: --publish [tcp/udp:]hostPort:containerPort"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "run [OPTIONS] IMAGE [COMMAND] [ARG...]\n\nCreate a pod, and launch a new VM to run the pod"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	var (
		podJson string
		attach  bool = false
	)

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
		podJson, err = cli.JsonFromCmdline(args[1:], opts.Env, opts.Portmap, opts.LogDriver, opts.LogOpts,
			opts.Name, opts.Workdir, opts.RestartPolicy, opts.Cpu, opts.Memory, opts.Tty)
	}

	if err != nil {
		return err
	}

	t1 := time.Now()

	var (
		spec  pod.UserPod
		async = true
	)
	json.Unmarshal([]byte(podJson), &spec)

	vmId, err := cli.CreateVm(spec.Resource.Vcpu, spec.Resource.Memory, async)

	podId, err := cli.CreatePod(podJson, opts.Remove)
	if err != nil {
		return err
	}

	fmt.Printf("POD id is %s\n", podId)

	_, err = cli.StartPod(podId, vmId, attach)
	if err != nil {
		return err
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

// cmdArgs: args[1:]
func (cli *HyperClient) JsonFromCmdline(cmdArgs, cmdEnvs, cmdPortmaps []string, cmdLogDriver string, cmdLogOpts []string,
	cmdName, cmdWorkdir, cmdRestartPolicy string, cpu, memory int, tty bool) (string, error) {

	var (
		name    = cmdName
		image   = cmdArgs[0]
		command = []string{}
		env     = []pod.UserEnvironmentVar{}
		ports   = []pod.UserContainerPort{}
		logOpts = make(map[string]string)
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

	containerList := []pod.UserContainer{{
		Name:          name,
		Image:         image,
		Command:       command,
		Workdir:       cmdWorkdir,
		Entrypoint:    []string{},
		Ports:         ports,
		Envs:          env,
		Volumes:       []pod.UserVolumeReference{},
		Files:         []pod.UserFileReference{},
		RestartPolicy: cmdRestartPolicy,
	}}

	userPod := &pod.UserPod{
		Name:       name,
		Containers: containerList,
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

func (cli *HyperClient) GetContainerByPod(podId string) (string, error) {
	v := url.Values{}
	v.Set("item", "container")
	v.Set("pod", podId)
	body, _, err := readBody(cli.call("GET", "/list?"+v.Encode(), nil, nil))
	if err != nil {
		return "", err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return "", err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return "", err
	}
	out.Close()
	var containerResponse = []string{}
	containerResponse = remoteInfo.GetList("cData")
	for _, c := range containerResponse {
		fields := strings.Split(c, ":")
		containerId := fields[0]
		if podId == fields[2] {
			return containerId, nil
		}
	}

	return "", fmt.Errorf("Container not found")
}
