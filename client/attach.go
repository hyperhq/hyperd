package client

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/lib/promise"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdAttach(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "attach CONTAINER\n\nAttach to the tty of a specified container in a pod"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 1 {
		return fmt.Errorf("Can not accept the 'attach' command without Container ID!")
	}
	var (
		podName     = args[1]
		tag         = cli.GetTag()
		containerId = podName
	)

	v := url.Values{}
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
		v.Set("value", containerId)
	}
	v.Set("tag", tag)

	return cli.hijackRequest("attach", podName, tag, &v)
}

func (cli *HyperClient) hijackRequest(method, pod, tag string, v *url.Values) error {
	var (
		hijacked = make(chan io.Closer)
		errCh    chan error
	)
	// Block the return until the chan gets closed
	defer func() {
		fmt.Printf("End of CmdExec(), Waiting for hijack to finish.\n")
		if _, ok := <-hijacked; ok {
			fmt.Printf("Hijack did not finish (chan still open)\n")
		}
	}()

	request := fmt.Sprintf("/%s?%s", method, v.Encode())

	errCh = promise.Go(func() error {
		return cli.hijack("POST", request, true, cli.in, cli.out, cli.out, hijacked, nil, "")
	})

	if err := cli.monitorTtySize(pod, tag); err != nil {
		fmt.Printf("Monitor tty size fail for %s!\n", pod)
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
	fmt.Printf("Successfully attached to pod(%s)\n", pod)
	return nil
}
