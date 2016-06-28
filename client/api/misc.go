package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/docker/engine-api/types"
)

// An StatusError reports an unsuccessful exit by a command.
type StatusError struct {
	Status     string
	StatusCode int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("Status: %s, Code: %d", e.Status, e.StatusCode)
}

func (cli *Client) GetExitCode(containerId, execId string) error {
	v := url.Values{}
	v.Set("container", containerId)
	v.Set("exec", execId)
	code := -1

	stream, _, err := cli.call("GET", "/exitcode?"+v.Encode(), nil, nil)
	if err != nil {
		return err
	}
	defer stream.Close()

	err = json.NewDecoder(stream).Decode(&code)
	if err != nil {
		fmt.Printf("Error get exit code: %s", err.Error())
		return err
	}

	if code != 0 {
		return StatusError{StatusCode: code}
	}

	return nil
}

func (cli *Client) AuthHeader(orig map[string][]string, auth types.AuthConfig) (map[string][]string, error) {
	buf, err := json.Marshal(auth)
	if err != nil {
		return orig, err
	}
	registryAuthHeader := []string{
		base64.URLEncoding.EncodeToString(buf),
	}

	if orig == nil {
		orig = make(map[string][]string)
	}

	orig["X-Registry-Auth"] = registryAuthHeader
	return orig, nil
}

func (cli *Client) WinResize(containerId, execId string, height, width int) error {
	v := url.Values{}
	v.Set("h", strconv.Itoa(height))
	v.Set("w", strconv.Itoa(width))
	v.Set("container", containerId)
	v.Set("exec", execId)

	_, _, err := readBody(cli.call("POST", "/tty/resize?"+v.Encode(), nil, nil))

	return err
}
