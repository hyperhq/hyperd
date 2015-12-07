package client

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/lib/term"
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
			return fmt.Errorf("Cannot connect to the Hyper daemon. Is 'hyperd' running on this host?")
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

	rwc, _ := clientconn.Hijack()
	defer rwc.Close()

	if started != nil {
		started <- rwc
	}

	_, err = term.TtySplice(rwc)
	return err
}
