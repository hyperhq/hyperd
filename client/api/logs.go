package api

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func (cli *Client) ContainerLogs(container, since string, timestamp, follow, stdout, stderr bool, tail string) (io.ReadCloser, string, error) {
	v := url.Values{}
	v.Set("container", container)
	v.Set("stdout", strconv.FormatBool(stdout))
	v.Set("stderr", strconv.FormatBool(stderr))
	v.Set("since", since)
	v.Set("timestamps", strconv.FormatBool(timestamp))
	v.Set("follow", strconv.FormatBool(follow))
	v.Set("tail", tail)

	headers := http.Header(make(map[string][]string))
	out, contenttype, err := cli.stream("GET", "/container/logs?"+v.Encode(), nil, headers)
	if err != nil {
		return nil, "", err
	}
	return out, contenttype, nil
}
