package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperhq/runv/lib/term"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdExec(args ...string) error {
	var opts struct {
		Attach bool `short:"a" long:"attach" default:"true" description:"attach current terminal to the stdio of command"`
		Tty    bool `short:"t" long:"tty" description:"Allocate a pseudo-TTY"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "exec [OPTIONS] POD|CONTAINER COMMAND [ARGS...]\n\nRun a command in a container of a running pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("Can not accept the 'exec' command without Container ID!")
	}
	if len(args) == 1 {
		return fmt.Errorf("Can not accept the 'exec' command without command!")
	}
	var (
		podName     = args[0]
		containerId string
	)

	if strings.Contains(podName, "pod-") {
		containerId, err = cli.client.GetContainerByPod(podName)
		if err != nil {
			return err
		}
	} else {
		containerId = args[0]
	}

	command, err := json.Marshal(args[1:])
	if err != nil {
		return err
	}

	execId, err := cli.client.CreateExec(containerId, command, opts.Tty)
	if err != nil {
		return err
	}

	if opts.Tty {
		if err := cli.monitorTtySize(containerId, execId); err != nil {
			fmt.Printf("Monitor tty size fail for %s!\n", podName)
		}
		oldState, err := term.SetRawTerminal(cli.inFd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(cli.inFd, oldState)
	}

	err = cli.client.StartExec(containerId, execId, opts.Tty, cli.in, cli.out, cli.err)
	if err != nil {
		return err
	}

	return cli.client.GetExitCode(containerId, execId)
}
