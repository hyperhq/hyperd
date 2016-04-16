package api

import (
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/hyperhq/hyperd/lib/promise"
)

func (cli *Client) Exec(container, tag string, command []byte, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error {

	v := url.Values{}
	v.Set("type", "container")
	v.Set("value", container)
	v.Set("command", string(command))
	v.Set("tag", tag)
	v.Set("tty", strconv.FormatBool(tty))

	var (
		hijacked = make(chan io.Closer)
		errCh    chan error
	)
	// Block the return until the chan gets closed
	defer func() {
		//fmt.Printf("End of CmdExec(), Waiting for hijack to finish.\n")
		if _, ok := <-hijacked; ok {
			fmt.Printf("Hijack did not finish (chan still open)\n")
		}
	}()

	errCh = promise.Go(func() error {
		return cli.hijack("POST", "/exec?"+v.Encode(), tty, stdin, stdout, stdout, hijacked, nil, "")
	})

	// Acknowledge the hijack before starting
	select {
	case closer := <-hijacked:
		// Make sure that hijack gets closed when returning. (result
		// in closing hijack chan and freeing server's goroutines.
		if closer != nil {
			defer closer.Close()
		}
	case err := <-errCh:
		if err != nil {
			fmt.Printf("Error hijack: %s", err.Error())
			return err
		}
	}

	if err := <-errCh; err != nil {
		fmt.Printf("Error hijack: %s", err.Error())
		return err
	}

	return nil
}
