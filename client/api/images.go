package api

import (
	"fmt"
	"net/url"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) GetImages(all, quiet bool) (*engine.Env, error) {

	v := url.Values{}
	v.Set("all", "no")
	v.Set("quiet", "no")
	if all == true {
		v.Set("all", "yes")
	}
	if quiet == true {
		v.Set("quiet", "yes")
	}
	body, _, err := readBody(cli.call("GET", "/images/get?"+v.Encode(), nil, nil))
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
