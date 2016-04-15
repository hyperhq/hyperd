package api

import (
	"encoding/json"
	"net/url"

	"github.com/hyperhq/hyperd/engine"
)

func (cli *Client) Commit(container, repo, author, message string, changes []string, pause bool) (string, error) {
	v := url.Values{}
	v.Set("author", author)
	changeJson, err := json.Marshal(changes)
	if err != nil {
		return "", err
	}
	v.Set("change", string(changeJson))
	v.Set("message", message)
	if pause == true {
		v.Set("pause", "yes")
	} else {
		v.Set("pause", "no")
	}
	v.Set("container", container)
	v.Set("repo", repo)
	body, _, err := readBody(cli.call("POST", "/container/commit?"+v.Encode(), nil, nil))
	if err != nil {
		return "", err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return "", err
	}

	if _, err := out.Write(body); err != nil {
		return "", err
	}
	out.Close()

	return remoteInfo.Get("ID"), nil
}
