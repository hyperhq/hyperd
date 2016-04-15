package api

import (
	"fmt"
	"net/url"

	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (cli *Client) RmPod(id string) error {
	v := url.Values{}
	v.Set("podId", id)
	body, _, err := readBody(cli.call("DELETE", "/pod?"+v.Encode(), nil, nil))
	if err != nil {
		return fmt.Errorf("Error to remove pod(%s), %s", id, err.Error())
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return fmt.Errorf("Error to remove pod(%s), %s", id, err.Error())
	}

	if _, err := out.Write(body); err != nil {
		return fmt.Errorf("Error to remove pod(%s), %s", id, err.Error())
	}
	out.Close()
	errCode := remoteInfo.GetInt("Code")
	if !(errCode == types.E_OK || errCode == types.E_VM_SHUTDOWN) {
		return fmt.Errorf("Error to remove pod(%s), %s", id, remoteInfo.Get("Cause"))
	}
	return nil
}
