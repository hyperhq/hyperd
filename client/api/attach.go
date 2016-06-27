package api

import (
	"fmt"
	"io"
	"net/url"
)

func (cli *Client) Attach(container string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error {
	v := url.Values{}
	v.Set("container", container)

	err := cli.hijackRequest("attach", &v, tty, stdin, stdout, stderr)
	if err != nil {
		fmt.Printf("attach failed: %s\n", err.Error())
		return err
	}
	return nil
}
