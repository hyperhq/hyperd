package api

import (
	"net/url"
	"strconv"
)

func (cli *Client) KillContainer(container string, sig int) error {
	v := url.Values{}
	v.Set("container", container)
	v.Set("signal", strconv.Itoa(sig))

	_, _, err := readBody(cli.call("POST", "/container/kill?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	return nil
}

func (cli *Client) KillPod(pod string, sig int) error {
	v := url.Values{}
	v.Set("podName", pod)
	v.Set("signal", strconv.Itoa(sig))

	_, _, err := readBody(cli.call("POST", "/pod/kill?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	return nil
}
