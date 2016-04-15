package api

import (
	"net/url"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) PausePod(podId string) error {
	v := url.Values{}
	v.Set("podId", podId)

	body, _, err := readBody(cli.call("POST", "/pod/pause?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}

	out := engine.NewOutput()
	if _, err = out.AddEnv(); err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return err
	}
	out.Close()

	return nil
}

func (cli *Client) UnpausePod(podId string) error {
	v := url.Values{}
	v.Set("podId", podId)

	body, _, err := readBody(cli.call("POST", "/pod/unpause?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}

	out := engine.NewOutput()
	if _, err = out.AddEnv(); err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return err
	}
	out.Close()

	return nil
}
