package api

import (
	"io"
)

func (cli *Client) Load(body io.Reader) (io.ReadCloser, string, error) {

	headers := map[string][]string{"Content-Type": {"application/x-tar"}}

	out, contenttype, err := cli.stream("POST", "/image/load", body, headers)
	if err != nil {
		return nil, "", err
	}
	return out, contenttype, nil
}
