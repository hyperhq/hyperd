package client

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"hyper/lib/promise"
	"hyper/lib/term"
	"hyper/utils"
)

func (cli *HyperClient) dial() (net.Conn, error) {
	return net.Dial(cli.proto, cli.addr)
}

func (cli *HyperClient) hijack(method, path string, setRawTerminal bool, in io.ReadCloser, stdout, stderr io.Writer, started chan io.Closer, data interface{}, hostname string) error {
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
			return fmt.Errorf("Cannot connect to the Docker daemon. Is 'docker' running on this host?")
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
		oldState      *term.State
	)

	if in != nil && setRawTerminal {
		// fmt.Printf("In the Raw Terminal!!!\n")
		oldState, err = term.SetRawTerminal(cli.inFd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(cli.inFd, oldState)
	}

	if stdout != nil || stderr != nil {
		receiveStdout = promise.Go(func() (err error) {
			defer func() {
				if in != nil {
					if setRawTerminal {
						term.RestoreTerminal(cli.inFd, oldState)
					}
				}
			}()

			_, err = io.Copy(stdout, br)
			// fmt.Printf("[hijack] End of stdout\n")
			return err
		})
	}

	sendStdin := promise.Go(func() error {
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
		// Discard errors due to pipe interruption
		return nil
	})

	if stdout != nil || stderr != nil {
		if err := <-receiveStdout; err != nil {
			fmt.Printf("Error receiveStdout: %s\n", err.Error())
			return err
		}
		sendStdin <- nil
	}
	if in != nil {
		if err := <-sendStdin; err != nil {
			fmt.Printf("Error sendStdin: %s\n", err.Error())
			return err
		}
	}

	return nil
}
