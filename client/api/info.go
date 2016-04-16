package api

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/hyperd/types"
)

func (cli *Client) Info() (*engine.Env, error) {

	body, _, err := readBody(cli.call("GET", "/info", nil, nil))
	if err != nil {
		return nil, err
	}

	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return nil, err
	}

	if _, err := out.Write(body); err != nil {
		return nil, err
	}
	out.Close()

	return remoteInfo, nil
}

func (cli *Client) GetPodInfo(podName string) (*types.PodInfo, error) {
	// get the pod or container info before we start the exec
	v := url.Values{}
	v.Set("podName", podName)
	body, _, err := readBody(cli.call("GET", "/pod/info?"+v.Encode(), nil, nil))
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return nil, err
	}
	var jsonData types.PodInfo
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return nil, err
	}

	return &jsonData, nil
}

func (cli *Client) GetContainerInfo(container string) (*types.ContainerInfo, error) {
	// get the pod or container info before we start the exec
	v := url.Values{}
	v.Set("container", container)
	body, _, err := readBody(cli.call("GET", "/container/info?"+v.Encode(), nil, nil))
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return nil, err
	}
	var jsonData types.ContainerInfo
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return nil, err
	}

	return &jsonData, nil
}
