package api

import (
	"fmt"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
	"net/http"
)

func (cli *Client) CreatePod(spec interface{}) (string, int, error) {
	body, statusCode, err := readBody(cli.call("POST", "/pod/create", spec, nil))
	if statusCode != http.StatusCreated && statusCode != http.StatusOK {
		return "", statusCode, err
	} else if err != nil {
		return "", statusCode, err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return "", statusCode, err
	}

	if _, err := out.Write(body); err != nil {
		return "", statusCode, fmt.Errorf("Error reading remote info: %s", err)
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
			return "", statusCode, fmt.Errorf("Error code is %d", errCode)
		} else {
			return "", statusCode, fmt.Errorf("Cause is %s", remoteInfo.Get("Cause"))
		}
	}
	return remoteInfo.Get("ID"), statusCode, nil
}
