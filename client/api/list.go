package api

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) GetContainerByPod(podId string) (string, error) {
	v := url.Values{}
	v.Set("item", "container")
	v.Set("pod", podId)
	body, _, err := readBody(cli.call("GET", "/list?"+v.Encode(), nil, nil))
	if err != nil {
		return "", err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return "", err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return "", err
	}
	out.Close()
	var containerResponse = []string{}
	containerResponse = remoteInfo.GetList("cData")
	for _, c := range containerResponse {
		fields := strings.Split(c, ":")
		containerId := fields[0]
		if podId == fields[2] {
			return containerId, nil
		}
	}

	return "", fmt.Errorf("Container not found")
}

func (cli *Client) List(item, pod, vm string, aux bool) (*engine.Env, error) {
	v := url.Values{}
	v.Set("item", item)
	if aux {
		v.Set("auxiliary", "yes")
	}
	if pod != "" {
		v.Set("pod", pod)
	}
	if vm != "" {
		v.Set("vm", vm)
	}
	body, _, err := readBody(cli.call("GET", "/list?"+v.Encode(), nil, nil))
	if err != nil {
		return nil, err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return nil, err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return nil, err
	}
	out.Close()

	return remoteInfo, nil
}
