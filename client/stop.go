package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/engine"
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
	code, cause, err := cli.StopPod(podID, stopVm)
	if err != nil {
		return err
	}
	if code != types.E_POD_STOPPED && code != types.E_VM_SHUTDOWN {
		return fmt.Errorf("Error code is %d, cause is %s", code, cause)
	}
	fmt.Printf("Successfully shutdown the POD: %s!\n", podID)
	return nil
}

func (cli *HyperClient) StopPod(podId, stopVm string) (int, string, error) {
	v := url.Values{}
	v.Set("podId", podId)
	v.Set("stopVm", stopVm)
	body, _, err := readBody(cli.call("POST", "/pod/stop?"+v.Encode(), nil, nil))
	if err != nil {
		if strings.Contains(err.Error(), "leveldb: not found") {
			return -1, "", fmt.Errorf("Can not find that POD ID to stop, please check your POD ID!")
		}
		return -1, "", err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return -1, "", err
	}

	if _, err := out.Write(body); err != nil {
		return -1, "", err
	}
	out.Close()
	// This 'ID' stands for pod ID
	// This 'Code' should be E_SHUTDOWN
	// THis 'Cause' ..
	if remoteInfo.Exists("ID") {
		// TODO ...
	}
	return remoteInfo.GetInt("Code"), remoteInfo.Get("Cause"), nil
}
