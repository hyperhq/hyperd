package api

import (
	"fmt"
	"io"
	"net/url"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (cli *Client) StartPod(podId, vmId, tag string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) (string, error) {
	var attach = false
	v := url.Values{}
	v.Set("podId", podId)
	v.Set("vmId", vmId)

	if tag != "" {
		attach = true
		v.Set("tag", tag)
	}

	if !attach {
		return cli.startPodWithoutTty(&v)
	} else {
		err := cli.hijackRequest("pod/start", tag, &v, tty, stdin, stdout, stderr)
		if err != nil {
			fmt.Printf("StartPod failed: %s\n", err.Error())
			return "", err
		}

		containerId, err := cli.GetContainerByPod(podId)
		if err != nil {
			return "", err
		}

		return "", cli.GetExitCode(containerId, tag)
	}
}

func (cli *Client) startPodWithoutTty(v *url.Values) (string, error) {

	body, _, err := readBody(cli.call("POST", "/pod/start?"+v.Encode(), nil, nil))
	if err != nil {
		return "", err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return "", err
	}

	if _, err := out.Write(body); err != nil {
		return "", fmt.Errorf("Error reading remote info: %s", err)
	}
	out.Close()
	errCode := remoteInfo.GetInt("Code")
	if errCode != types.E_OK {
		if errCode != types.E_BAD_REQUEST &&
			errCode != types.E_FAILED {
			return "", fmt.Errorf("Error code is %d", errCode)
		} else {
			return "", fmt.Errorf("Cause is %s", remoteInfo.Get("Cause"))
		}
	}
	return remoteInfo.Get("ID"), nil
}
