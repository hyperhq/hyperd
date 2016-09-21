package api

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/hyperhq/hyperd/lib/promise"
	"github.com/hyperhq/hyperd/utils"
)

func (cli *Client) hijackRequest(method string, v *url.Values, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error {
	var (
		hijacked = make(chan io.Closer)
		errCh    chan error
	)
	// Block the return until the chan gets closed
	defer func() {
		if _, ok := <-hijacked; ok {
			fmt.Printf("Hijack did not finish (chan still open)\n")
		}
	}()

	request := fmt.Sprintf("/%s?%s", method, v.Encode())

	errCh = promise.Go(func() error {
		return cli.hijack("POST", request, tty, stdin, stdout, stderr, hijacked, nil, "")
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

func (cli *Client) dial() (net.Conn, error) {
	return net.Dial(cli.proto, cli.addr)
}

func (cli *Client) hijack(method, path string, setRawTerminal bool, in io.ReadCloser, stdout, stderr io.Writer, started chan io.Closer, data interface{}, hostname string) error {
	defer func() {
		if started != nil {
			close(started)
		}
	}()

	params, err := cli.encodeData(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, fmt.Sprintf("/v%s%s", utils.APIVERSION, path), params)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Hyper-Client/"+utils.VERSION)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	req.Host = cli.addr

	dial, err := cli.dial()
	if err != nil {
		return err
	}
	// When we set up a TCP connection for hijack, there could be long periods
	// of inactivity (a long running command with no output) that in certain
	// network setups may cause ECONNTIMEOUT, leaving the client in an unknown
	// state. Setting TCP KeepAlive on the socket connection will prohibit
	// ECONNTIMEOUT unless the socket connection truly is broken
	if tcpConn, ok := dial.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return ErrConnectionRefused
		}
		return err
	}
	clientconn := httputil.NewClientConn(dial, nil)
	defer clientconn.Close()

	// Server hijacks the connection, error 'connection closed' expected
	_, err = clientconn.Do(req)
	if err != nil {
		fmt.Printf("Client DO: %s\n", err.Error())
	}

	rwc, br := clientconn.Hijack()
	defer rwc.Close()

	if started != nil {
		started <- rwc
	}

	var (
		receiveStdout chan error
	)

	if in != nil && setRawTerminal {
		// fmt.Printf("In the Raw Terminal!!!\n")
	}

	if stdout != nil || stderr != nil {
		receiveStdout = promise.Go(func() (err error) {
			defer func() {
				if in != nil {
					if setRawTerminal {
					}
				}
			}()

			if !setRawTerminal {
				_, err = stdcopy.StdCopy(stdout, stderr, br)
			} else {
				_, err = io.Copy(stdout, br)
			}
			// fmt.Printf("[hijack] End of stdout\n")
			return err
		})
	}

	go func() {
		if in != nil {
			io.Copy(rwc, in)
			// fmt.Printf("[hijack] End of stdin\n")
		}

		if conn, ok := rwc.(interface {
			CloseWrite() error
		}); ok {
			if err := conn.CloseWrite(); err != nil {
				fmt.Printf("Couldn't send EOF: %s", err.Error())
			}
		}
	}()

	if stdout != nil || stderr != nil {
		if err := <-receiveStdout; err != nil {
			fmt.Printf("Error receiveStdout: %s\n", err.Error())
			return err
		}
	}

	return nil
}
