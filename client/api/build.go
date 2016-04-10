package api

import (
	"io"
	"net/http"
	"net/url"
)

func (cli *Client) Build(name string, hasBody bool, body io.Reader) (io.ReadCloser, string, error) {
	v := url.Values{}
	v.Set("name", name)
	headers := http.Header(make(map[string][]string))
	if hasBody {
		headers.Set("Content-Type", "application/tar")
	}
	out, contenttype, err := cli.stream("POST", "/image/build?"+v.Encode(), body, headers)
	if err != nil {
		return nil, "", err
	}
	return out, contenttype, nil
}
