package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/docker/engine-api/types"
	"net/url"
)

func (cli *Client) Push(tag, repo string, authConfig types.AuthConfig) (io.ReadCloser, string, int, error) {

	v := url.Values{}
	v.Set("tag", tag)
	v.Set("remote", repo)

	body, ctype, statusCode, err := cli.authRequest("POST", "/image/push?"+v.Encode(), nil, nil, authConfig)
	if err != nil {
		// Since errors in a stream appear after status 200 has been written,
		// we may need to change the status code.
		if strings.Contains(err.Error(), "Authentication is required") ||
			strings.Contains(err.Error(), "Status 401") ||
			strings.Contains(err.Error(), "status code 401") {
			statusCode = http.StatusUnauthorized
		}
	}
	return body, ctype, statusCode, err
}
