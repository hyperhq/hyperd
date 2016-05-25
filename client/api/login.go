package api

import (
	"encoding/json"

	"github.com/docker/engine-api/types"
)

func (cli *Client) Login(auth types.AuthConfig, response *types.AuthResponse) (remove bool, err error) {
	stream, statusCode, err := cli.call("POST", "/auth", auth, nil)
	if err != nil {
		return false, err
	}
	defer stream.Close()

	if statusCode == 401 {
		return true, err
	}

	if err := json.NewDecoder(stream).Decode(response); err != nil {
		return true, err
	}
	return false, nil
}
