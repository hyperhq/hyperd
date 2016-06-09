package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor/types"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdStop(args ...string) error {

	var opts struct {
		Container bool `short:"c" long:"container" default:"false" default-mask:"-" description:"stop container"`
		Novm      bool `long:"onlypod" default:"false" description:"Stop a Pod, but left the VM running"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "stop [OPTIONS] CONTAINER_ID|POD_ID\n\nStop running container or pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"stop\" requires a minimum of 1 argument, please provide POD ID.\n")
	}

	stopVm := "yes"
	if opts.Novm {
		stopVm = "no"
	}

	for i := range args {
		if opts.Container {
			err = cli.client.StopContainer(args[i])
			if err != nil {
				fmt.Fprintf(cli.err, "fail to stop container %s: %v", args[i], err)
			}
		} else {
			code, cause, err := cli.client.StopPod(args[i], stopVm)
			if err != nil {
				fmt.Fprintf(cli.err, "fail to stop pod %s: %v", args[i], err)
				continue
			}
			if code != types.E_POD_STOPPED && code != types.E_VM_SHUTDOWN {
				fmt.Fprintf(cli.err, "Error code is %d, cause is %s", code, cause)
				continue
			}
			fmt.Printf("Successfully shutdown the POD: %s!\n", args[i])
		}
	}

	return nil
}
