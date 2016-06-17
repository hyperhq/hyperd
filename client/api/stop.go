package api

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) StopContainer(container string) error {
	v := url.Values{}
	v.Set("container", container)

	_, _, err := readBody(cli.call("POST", "/container/stop?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	return nil
}

func (cli *Client) StopPod(podId, stopVm string) (int, string, error) {
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
