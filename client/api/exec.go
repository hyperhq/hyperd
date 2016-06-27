package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hyperhq/hyperd/lib/promise"
)

func (cli *Client) CreateExec(container string, command []byte, tty bool) (string, error) {
	var execId string

	v := url.Values{}
	v.Set("container", container)
	v.Set("command", string(command))
	v.Set("tty", strconv.FormatBool(tty))

	body, statusCode, err := readBody(cli.call("POST", "/exec/create?"+v.Encode(), nil, nil))
	if err != nil {
		return "", err
	}

	if statusCode != http.StatusCreated && statusCode != http.StatusOK {
		return "", err
	}

	err = json.Unmarshal(body, &execId)
	if err != nil {
		return "", err
	}

	return execId, nil
}

func (cli *Client) StartExec(containerId, execId string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error {

	v := url.Values{}
	v.Set("container", containerId)
	v.Set("exec", execId)

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
		return cli.hijack("POST", "/exec/start?"+v.Encode(), tty, stdin, stdout, stdout, hijacked, nil, "")
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
