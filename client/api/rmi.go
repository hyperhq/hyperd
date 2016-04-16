package api

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) RemoveImage(image string, noprune, force bool) (*engine.Env, error) {
	v := url.Values{}
	v.Set("imageId", image)
	v.Set("noprune", strconv.FormatBool(noprune))
	v.Set("force", strconv.FormatBool(force))
	body, _, err := readBody(cli.call("DELETE", "/image?"+v.Encode(), nil, nil))
	if err != nil {
		return nil, fmt.Errorf("Error remove the image(%s): %s", image, err.Error())
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return nil, fmt.Errorf("Error remove the image(%s): %s", image, err.Error())
	}

	if _, err := out.Write(body); err != nil {
		return nil, fmt.Errorf("Error remove the image(%s): %s", image, err.Error())
	}
	out.Close()

	return remoteInfo, nil
}
