package client

import (
	"encoding/json"
	"fmt"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/lib/promise"
	"io"
	"net/url"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdExec(args ...string) error {
	var opts struct {
		Attach bool `short:"a" long:"attach" default:"true" value-name:"false" description:"attach current terminal to the stdio of command"`
		Vm     bool `long:"vm" default:"false" value-name:"false" description:"attach to vm"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "exec [OPTIONS] POD|CONTAINER COMMAND [ARGS...]\n\nrun a command in a container of a running pod"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 1 {
		return fmt.Errorf("Can not accept the 'exec' command without POD/Container ID!")
	}
	if len(args) == 2 {
		return fmt.Errorf("Can not accept the 'exec' command without command!")
	}
	var (
		podName     = args[1]
		hostname    = ""
		tag         = cli.GetTag()
		containerId string
	)
	command, err := json.Marshal(args[2:])
	if err != nil {
		return err
	}
	// fmt.Printf("The pod name is %s, command is %s\n", podName, string(command))

	v := url.Values{}
	if opts.Vm == true {
		v.Set("type", "pod")
		v.Set("value", podName)
	} else {
		if strings.Contains(podName, "pod-") {
			hostname, err = cli.GetPodInfo(podName)
			if err != nil {
				return err
			}
			if hostname == "" {
				return fmt.Errorf("The POD : %s does not exist, please create it before exec!", podName)
			}
			containerId, err = cli.GetContainerByPod(podName)
			if err != nil {
				return err
			}
			v.Set("type", "container")
			v.Set("value", containerId)
		} else {
			v.Set("type", "container")
			v.Set("value", podName)
		}
	}
	v.Set("command", string(command))
	v.Set("tag", tag)

	var (
		hijacked = make(chan io.Closer)
		errCh    chan error
	)
	// Block the return until the chan gets closed
	defer func() {
		//fmt.Printf("End of CmdExec(), Waiting for hijack to finish.\n")
		if _, ok := <-hijacked; ok {
			fmt.Printf("Hijack did not finish (chan still open)\n")
		}
	}()

	errCh = promise.Go(func() error {
		return cli.hijack("POST", "/exec?"+v.Encode(), true, cli.in, cli.out, cli.out, hijacked, nil, hostname)
	})

	if err := cli.monitorTtySize(podName, tag); err != nil {
		fmt.Printf("Monitor tty size fail for %s!\n", podName)
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
	//fmt.Printf("Success to exec the command %s for POD %s!\n", command, podName)
	return nil
}

func (cli *HyperClient) GetPodInfo(podName string) (string, error) {
	// get the pod or container info before we start the exec
	v := url.Values{}
	v.Set("podName", podName)
	body, _, err := readBody(cli.call("GET", "/pod/info?"+v.Encode(), nil, nil))
	if err != nil {
		fmt.Printf("Error: %s\n", err)
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
	if remoteInfo.Exists("hostname") {
		hostname := remoteInfo.Get("hostname")
		if hostname == "" {
			return "", nil
		} else {
			return hostname, nil
		}
	}

	return "", nil
}
