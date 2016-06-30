package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/lib/term"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdAttach(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "attach CONTAINER\n\nAttach to the tty of a specified container in a pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("Can not accept the 'attach' command without Container ID!")
	}
	var (
		podId       = args[0]
		containerId = args[0]
		tty         bool
	)

	if strings.Contains(podId, "pod-") {
		pod, err := cli.client.GetPodInfo(podId)
		if err != nil {
			return err
		}

		if len(pod.Spec.Containers) == 0 {
			return fmt.Errorf("failed to get container from %s", podId)
		}

		c := pod.Spec.Containers[0]

		containerId = c.ContainerID
		tty = c.Tty
	} else {
		c, err := cli.client.GetContainerInfo(containerId)
		if err != nil {
			return err
		}

		podId = c.PodID
		containerId = c.Container.ContainerID
		tty = c.Container.Tty
	}

	if tty {
		oldState, err := term.SetRawTerminal(cli.inFd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(cli.inFd, oldState)
		cli.monitorTtySize(containerId, "")
	}

	if err := cli.client.Attach(containerId, tty, cli.in, cli.out, cli.err); err != nil {
		return err
	}

	return cli.client.GetExitCode(containerId, "")
}
