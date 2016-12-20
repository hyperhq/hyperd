package api

import (
	"net/url"
)

func (cli *Client) StartPod(podId string) error {
	v := url.Values{}
	v.Set("podId", podId)

	_, _, err := readBody(cli.call("POST", "/pod/start?"+v.Encode(), nil, nil))
	if err != nil {
		return err
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
