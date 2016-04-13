package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor/types"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdStop(args ...string) error {

	var opts struct {
		Novm bool `long:"onlypod" default:"false" description:"Stop a Pod, but left the VM running"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "stop POD_ID\n\nStop a running pod"
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

	podID := args[0]
	stopVm := "yes"
	if opts.Novm {
		stopVm = "no"
	}
	code, cause, err := cli.client.StopPod(podID, stopVm)
	if err != nil {
		return err
	}
	if code != types.E_POD_STOPPED && code != types.E_VM_SHUTDOWN {
		return fmt.Errorf("Error code is %d, cause is %s", code, cause)
	}
	fmt.Printf("Successfully shutdown the POD: %s!\n", podID)
	return nil
}
