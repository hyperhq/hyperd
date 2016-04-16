package api

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (cli *Client) CreateVm(cpu, mem int, async bool) (id string, err error) {
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
		return id, err
	}

	out := engine.NewOutput()
	remoteInfo, err = out.AddEnv()
	if err != nil {
		return id, err
	}

	if _, err = out.Write(body); err != nil {
		err = fmt.Errorf("Error reading remote info: %v", err)
		return id, err
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

	return id, err
}

func (cli *Client) RmVm(vm string) (err error) {
	var (
		body       []byte
		remoteInfo *engine.Env
	)

	v := url.Values{}
	v.Set("vm", vm)
	body, _, err = readBody(cli.call("DELETE", "/vm?"+v.Encode(), nil, nil))
	if err != nil {
		return fmt.Errorf("Error to remove vm(%s), %s", vm, err.Error())
	}

	out := engine.NewOutput()
	remoteInfo, err = out.AddEnv()
	if err != nil {
		return err
	}

	if _, err = out.Write(body); err != nil {
		err = fmt.Errorf("Error reading remote info: %v", err)
		return err
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
	}

	return err
}
