package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/engine"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdKill(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "kill VM_ID\n\nterminate a VM instance"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 1 {
		return fmt.Errorf("\"kill\" requires a minimum of 1 argument, please provide VM ID.\n")
	}

	vmId := args[1]
	v := url.Values{}
	v.Set("vm", vmId)
	body, _, err := readBody(cli.call("POST", "/vm/kill?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return err
	}
	out.Close()
	if remoteInfo.Exists("ID") {
		// TODO ...
	}

	return nil
}
