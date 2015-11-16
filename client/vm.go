package client

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"

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

	id, err := cli.CreateVm(0, 0, false)
	if err != nil {
		return err
	}

	fmt.Printf("New VM id is %s\n", id)

	return nil
}

func (cli *HyperClient) CreateVm(cpu, mem int, async bool) (id string, err error) {
	id = ""
	err = nil
	var (
		body       []byte
		remoteInfo *engine.Env
	)

	v := url.Values{}
	if cpu > 0 {
		v.Set("cpu", strconv.Itoa(cpu))
	}
	if mem > 0 {
		v.Set("mem", strconv.Itoa(mem))
	}
	if async {
		v.Set("async", "yes")
	}

	body, _, err = readBody(cli.call("POST", "/vm/create?"+v.Encode(), nil, nil))
	if err != nil {
		return
	}

	out := engine.NewOutput()
	remoteInfo, err = out.AddEnv()
	if err != nil {
		return
	}

	if _, err = out.Write(body); err != nil {
		err = fmt.Errorf("Error reading remote info: %v", err)
		return
	}
	out.Close()
	errCode := remoteInfo.GetInt("Code")
	if errCode != types.E_OK {
		if errCode != types.E_BAD_REQUEST &&
			errCode != types.E_FAILED {
			err = fmt.Errorf("Error code is %d", errCode)
		} else {
			err = fmt.Errorf("Cause is %s", remoteInfo.Get("Cause"))
		}
	} else {
		id = remoteInfo.Get("ID")
	}

	return
}
