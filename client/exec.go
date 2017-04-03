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
		Detach bool `short:"d" long:"detach" default-mask:"-" description:"Not Attach the stdin, stdout and stderr to the process"`
		Tty    bool `short:"t" long:"tty" description:"Allocate a pseudo-TTY"`
		VM     bool `short:"m" long:"vm" description:"Execute outside of any containers"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown|gflag.PassAfterNonOption)
	parser.Usage = "exec [OPTIONS] POD|CONTAINER COMMAND [ARGS...]\n\nRun a command in a container or a Pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("Can not accept the 'exec' command without Container ID and command!")
	}
	if len(args) == 1 && !opts.VM {
		return fmt.Errorf("Can not accept the 'exec' command without command!")
	}

	command, err := json.Marshal(args[1:])
	if err != nil {
		return err
	}
	if opts.VM {
		return cli.client.ExecVM(args[0], command, cli.in, cli.out, cli.err)
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

	return cli.client.GetExitCode(containerId, execId, !opts.Tty)
}
