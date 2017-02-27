package api

import (
	"encoding/json"
	"io"
	"net/url"
)

func (cli *Client) Save(imageIDs []string, format string, refs map[string]string) (io.ReadCloser, error) {
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		return nil, err
	}

	v := url.Values{
		"names":  imageIDs,
		"format": []string{format},
		"refs":   []string{string(refsJSON)},
	}

	resp, _, err := cli.call("GET", "/images/save?"+v.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
