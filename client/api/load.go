package api

import (
	"encoding/json"
	"io"
	"net/url"
)

func (cli *Client) Load(body io.Reader, name string, refs map[string]string) (io.ReadCloser, string, error) {

	headers := map[string][]string{"Content-Type": {"application/x-tar"}}
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		return nil, "", err
	}

	v := url.Values{
		"name": []string{name},
		"refs": []string{string(refsJSON)},
	}

	out, contenttype, err := cli.stream("POST", "/image/load?"+v.Encode(), body, headers)
	if err != nil {
		return nil, "", err
	}
	return out, contenttype, nil
}
