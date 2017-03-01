package api

import (
	"io"
	"net/url"
)

func (cli *Client) Save(imageIDs []string) (io.ReadCloser, error) {
	v := url.Values{
		"names": imageIDs,
	}

	resp, _, err := cli.call("GET", "/images/save?"+v.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
