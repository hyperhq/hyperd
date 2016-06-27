package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdStart(args ...string) error {
	var opts struct {
		Cpu int `short:"c" long:"cpu" default:"1" value-name:"1" description:"CPU number for the VM"`
		Mem int `short:"m" long:"memory" default:"128" value-name:"128" description:"Memory size (MB) for the VM"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "start [-c 1 -m 128]| POD_ID \n\nLaunch a 'pending' pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"start\" requires a minimum of 1 argument, please provide POD ID.\n")
	}
	var (
		podId = args[0]
		vmId  string
	)
	if len(args) >= 2 {
		vmId = args[1]
	}

	_, err = cli.client.StartPod(podId, vmId, false, false, nil, nil, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(cli.out, "Successfully started the Pod(%s)\n", podId)
	return nil
}
