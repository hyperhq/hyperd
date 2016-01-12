package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/lib/promise"
	"github.com/hyperhq/hyper/types"

	gflag "github.com/jessevdk/go-flags"
)

// An StatusError reports an unsuccessful exit by a command.
type StatusError struct {
	Status     string
	StatusCode int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("Status: %s, Code: %d", e.Status, e.StatusCode)
}

func GetExitCode(cli *HyperClient, container, tag string) error {
	v := url.Values{}
	v.Set("container", container)
	v.Set("tag", tag)
	code := -1

	stream, _, err := cli.call("GET", "/exitcode?"+v.Encode(), nil, nil)
	if err != nil {
		return err
	}

	err = json.NewDecoder(stream).Decode(&code)
	if err != nil {
		fmt.Printf("Error get exit code: %s", err.Error())
		return err
	}

	if code != 0 {
		return StatusError{StatusCode: code}
	}

	return nil
}

func (cli *HyperClient) HyperCmdExec(args ...string) error {
	var opts struct {
		Attach bool `short:"a" long:"attach" default:"true" value-name:"false" description:"attach current terminal to the stdio of command"`
		Vm     bool `long:"vm" default:"false" value-name:"false" description:"attach to vm"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "exec [OPTIONS] POD|CONTAINER COMMAND [ARGS...]\n\nRun a command in a container of a running pod"
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
			_, err = cli.GetPodInfo(podName)
			if err != nil {
				return err
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
		return cli.hijack("POST", "/exec?"+v.Encode(), true, cli.in, cli.out, cli.out, hijacked, nil, "")
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

	return GetExitCode(cli, containerId, tag)
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
	var jsonData types.PodInfo
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return "", err
	}

	return jsonData.Vm, nil
}
