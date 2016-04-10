package api

import (
	"fmt"
	"io"
	"net/url"
)

func (cli *Client) Attach(container, termTag string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error {
	v := url.Values{}
	v.Set("type", "container")
	v.Set("value", container)
	v.Set("tag", termTag)

	err := cli.hijackRequest("attach", termTag, &v, tty, stdin, stdout, stderr)
	if err != nil {
		fmt.Printf("attach failed: %s\n", err.Error())
		return err
	}
	return nil
}
