package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/types"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdVm(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "vm\n\nRun a VM, without any Pod running on it"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	// Only run a new VM
	body, _, err := readBody(cli.call("POST", "/vm/create", nil, nil))
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return fmt.Errorf("Error reading remote info: %s", err)
	}
	out.Close()
	errCode := remoteInfo.GetInt("Code")
	if errCode == types.E_OK {
		//fmt.Println("VM is successful to start!")
	} else {
		// case types.E_CONTEXT_INIT_FAIL:
		// case types.E_DEVICE_FAIL:
		// case types.E_QMP_INIT_FAIL:
		// case types.E_QMP_COMMAND_FAIL:
		if errCode != types.E_BAD_REQUEST &&
			errCode != types.E_FAILED {
			return fmt.Errorf("Error code is %d", errCode)
		} else {
			return fmt.Errorf("Cause is %s", remoteInfo.Get("Cause"))
		}
	}
	fmt.Printf("New VM id is %s\n", remoteInfo.Get("ID"))
	return nil
}
