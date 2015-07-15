package client

import (
	"encoding/json"
	"fmt"
	gflag "github.com/jessevdk/go-flags"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/promise"
	"github.com/hyperhq/hyper/pod"
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
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "run [OPTIONS] IMAGE [COMMAND] [ARG...]\n\ncreate a pod, and launch a new VM to run the pod"
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
		podId, err := cli.RunPod(string(jsonbody))
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
		podId, err := cli.RunPod(string(jsonbody))
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
	)
	if len(args) > 2 {
		command = args[2:]
	}
	if opts.Name == "" {
		fields := strings.Split(image, ":")
		if len(fields) < 2 {
			opts.Name = image + "-" + pod.RandStr(10, "number")
		} else {
			opts.Name = fields[0] + "-" + fields[1] + "-" + pod.RandStr(10, "number")
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

	var containerList = []pod.UserContainer{}
	var container = pod.UserContainer{
		Name:          opts.Name,
		Image:         image,
		Command:       command,
		Workdir:       opts.Workdir,
		Entrypoint:    []string{},
		Ports:         []pod.UserContainerPort{},
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
	podId, err := cli.RunPod(string(jsonString))
	if err != nil {
		return err
	}
	fmt.Printf("POD id is %s\n", podId)
	// Get the container ID of this POD
	containerId, err := cli.GetContainerByPod(podId)
	if err != nil {
		return err
	}
	var (
		tag      = cli.GetTag()
		hijacked = make(chan io.Closer)
		errCh    chan error
	)
	v := url.Values{}
	v.Set("type", "container")
	v.Set("value", containerId)
	v.Set("tag", tag)

	// Block the return until the chan gets closed
	defer func() {
		// fmt.Printf("End of CmdExec(), Waiting for hijack to finish.\n")
		if _, ok := <-hijacked; ok {
			fmt.Printf("Hijack did not finish (chan still open)\n")
		}
	}()

	errCh = promise.Go(func() error {
		return cli.hijack("POST", "/attach?"+v.Encode(), true, cli.in, cli.out, cli.out, hijacked, nil, "")
	})

	if err := cli.monitorTtySize(podId, tag); err != nil {
		fmt.Printf("Monitor tty size fail for %s!\n", podId)
	}

	// Acknowledge the hijack before starting
	select {
	case closer := <-hijacked:
		// Make sure that hijack gets closed when returning. (result
		// in closing hijack chan and freeing server's goroutines.
		if closer != nil {
			defer closer.Close()
		}
	case err := <-errCh:
		if err != nil {
			fmt.Printf("Error hijack: %s", err.Error())
			return err
		}
	}

	if err := <-errCh; err != nil {
		fmt.Printf("Error hijack: %s", err.Error())
		return err
	}
	// fmt.Printf("Success to exec the command %s for POD %s!\n", command, podId)
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
		if podId == fields[1] {
			return containerId, nil
		}
	}

	return "", fmt.Errorf("Container not found")
}
