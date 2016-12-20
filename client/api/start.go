package api

import (
	"fmt"
	"net/url"

	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (cli *Client) StartPod(podId string) error {
	v := url.Values{}
	v.Set("podId", podId)

	body, _, err := readBody(cli.call("POST", "/pod/start?"+v.Encode(), nil, nil))
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
	if errCode != types.E_OK {
		if errCode != types.E_BAD_REQUEST &&
			errCode != types.E_FAILED {
			return fmt.Errorf("Error code is %d", errCode)
		} else {
			return fmt.Errorf("Cause is %s", remoteInfo.Get("Cause"))
		}
	}
	return nil
}

func (cli *Client) StartContainer(container string) error {
	v := url.Values{}
	v.Set("container", container)

	_, _, err := readBody(cli.call("POST", "/container/start?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	return nil

}
