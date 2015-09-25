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
		Attach        bool     `long:"attach" default:"true" default-mask:"-" description:"Attach the stdin, stdout and stderr to the container"`
		Workdir       string   `long:"workdir" default:"/" value-name:"\"\"" default-mask:"-" description:"Working directory inside the container"`
		Tty           bool     `long:"tty" default:"true" default-mask:"-" description:"Allocate a pseudo-TTY"`
		Cpu           int      `long:"cpu" default:"1" value-name:"1" default-mask:"-" description:"CPU number for the VM"`
		Memory        int      `long:"memory" default:"128" value-name:"128" default-mask:"-" description:"Memory size (MB) for the VM"`
		Env           []string `long:"env" value-name:"[]" default-mask:"-" description:"Set environment variables"`
		EntryPoint    string   `long:"entrypoint" value-name:"\"\"" default-mask:"-" description:"Overwrite the default ENTRYPOINT of the image"`
		RestartPolicy string   `long:"restart" default:"never" value-name:"\"\"" default-mask:"-" description:"Restart policy to apply when a container exits (never, onFailure, always)"`
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
	if opts.PodFile != "" {
		if _, err := os.Stat(opts.PodFile); err != nil {
			return err
		}

		jsonbody, err := ioutil.ReadFile(opts.PodFile)
		if err != nil {
			return err
		}

		if opts.Yaml == true {
			jsonbody, err = cli.ConvertYamlToJson(jsonbody)
			if err != nil {
				return err
			}
		}
		t1 := time.Now()
		podId, err := cli.RunPod(string(jsonbody), opts.Remove)
		if err != nil {
			return err
		}
		fmt.Printf("POD id is %s\n", podId)
		t2 := time.Now()
		fmt.Printf("Time to run a POD is %d ms\n", (t2.UnixNano()-t1.UnixNano())/1000000)
		return nil
	}
	if opts.K8s != "" {
		var (
			kpod    pod.KPod
			userpod *pod.UserPod
		)
		if _, err := os.Stat(opts.K8s); err != nil {
			return err
		}

		jsonbody, err := ioutil.ReadFile(opts.K8s)
		if err != nil {
			return err
		}
		if opts.Yaml == true {
			jsonbody, err = cli.ConvertYamlToJson(jsonbody)
			if err != nil {
				return err
			}
		}
		if err := json.Unmarshal(jsonbody, &kpod); err != nil {
			return err
		}
		userpod, err = kpod.Convert()
		if err != nil {
			return err
		}
		jsonbody, err = json.Marshal(*userpod)
		if err != nil {
			return err
		}
		t1 := time.Now()
		podId, err := cli.RunPod(string(jsonbody), opts.Remove)
		if err != nil {
			return err
		}
		fmt.Printf("POD id is %s\n", podId)
		t2 := time.Now()
		fmt.Printf("Time to run a POD is %d ms\n", (t2.UnixNano()-t1.UnixNano())/1000000)
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("%s: \"run\" requires a minimum of 1 argument, please provide the image.", os.Args[0])
	}
	var (
		image   = args[1]
		command = []string{}
		env     = []pod.UserEnvironmentVar{}
		ports   = []pod.UserContainerPort{}
		proto   string
		hPort   string
		cPort   string
	)
	if len(args) > 1 {
		command = args[2:]
	}
	if opts.Name == "" {
		opts.Name = image
		fields := strings.Split(image, "/")
		if len(fields) > 1 {
			opts.Name = fields[len(fields)-1]
		}
		fields = strings.Split(opts.Name, ":")
		if len(fields) < 2 {
			opts.Name = opts.Name + "-" + utils.RandStr(10, "number")
		} else {
			opts.Name = fields[0] + "-" + fields[1] + "-" + utils.RandStr(10, "number")
		}

		validContainerNameChars := `[a-zA-Z0-9][a-zA-Z0-9_.-]`
		validContainerNamePattern := regexp.MustCompile(`^/?` + validContainerNameChars + `+$`)
		if !validContainerNamePattern.MatchString(opts.Name) {
			opts.Name = namesgenerator.GetRandomName(0)
		}
	}
	if opts.Memory == 0 {
		opts.Memory = 128
	}
	if opts.Cpu == 0 {
		opts.Cpu = 1
	}
	for _, v := range opts.Env {
		if v == "" || !strings.Contains(v, "=") {
			continue
		}
		userEnv := pod.UserEnvironmentVar{
			Env:   v[:strings.Index(v, "=")],
			Value: v[strings.Index(v, "=")+1:],
		}
		env = append(env, userEnv)
	}

	for _, v := range opts.Portmap {
		port := pod.UserContainerPort{}
		fields := strings.Split(v, ":")
		if len(fields) < 2 {
			return fmt.Errorf("flag needs host port and container port: --publish")
		} else if len(fields) == 2 {
			proto = "tcp"
			hPort = fields[0]
			cPort = fields[1]
		} else {
			proto = fields[0]
			if proto != "tcp" && proto != "udp" {
				return fmt.Errorf("flag needs protocol(tcp or udp): --publish")
			}
			hPort = fields[1]
			cPort = fields[2]
		}

		port.Protocol = proto
		port.HostPort, err = strconv.Atoi(hPort)
		if err != nil {
			return fmt.Errorf("flag needs host port and container port: --publish")
		}
		port.ContainerPort, err = strconv.Atoi(cPort)
		if err != nil {
			return fmt.Errorf("flag needs host port and container port: --publish")
		}
		ports = append(ports, port)
	}

	var containerList = []pod.UserContainer{}
	var container = pod.UserContainer{
		Name:          opts.Name,
		Image:         image,
		Command:       command,
		Workdir:       opts.Workdir,
		Entrypoint:    []string{},
		Ports:         ports,
		Envs:          env,
		Volumes:       []pod.UserVolumeReference{},
		Files:         []pod.UserFileReference{},
		RestartPolicy: opts.RestartPolicy,
	}
	containerList = append(containerList, container)

	var userPod = &pod.UserPod{
		Name:       opts.Name,
		Containers: containerList,
		Resource:   pod.UserResource{Vcpu: opts.Cpu, Memory: opts.Memory},
		Files:      []pod.UserFile{},
		Volumes:    []pod.UserVolume{},
		Tty:        opts.Tty,
	}

	jsonString, _ := json.Marshal(userPod)

	podId, err := cli.CreatePod(string(jsonString))
	if err != nil {
		return err
	}

	if opts.Remove {
		defer func() { cli.HyperCmdRm(podId) }()
	}

	_, err = cli.StartPod(podId, "", true)
	if err != nil {
		return err
	}

	fmt.Printf("POD id is %s\n", podId)
	return nil
}

func (cli *HyperClient) GetContainerByPod(podId string) (string, error) {
	v := url.Values{}
	v.Set("item", "container")
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
