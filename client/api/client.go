package api

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

type Client struct {
	proto     string
	addr      string
	scheme    string
	tlsConfig *tls.Config
	transport *http.Transport
}

func NewClient(proto, addr string, tlsConfig *tls.Config) *Client {
	var (
		scheme = "http"
	)

	if tlsConfig != nil {
		scheme = "https"
	}

	// The transport is created here for reuse during the client session
	tran := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	// Why 32? See issue 8035
	timeout := 32 * time.Second
	if proto == "unix" {
		// no need in compressing for local communications
		tran.DisableCompression = true
		tran.Dial = func(_, _ string) (net.Conn, error) {
			return net.DialTimeout(proto, addr, timeout)
		}
	} else {
		tran.Proxy = http.ProxyFromEnvironment
		tran.Dial = (&net.Dialer{Timeout: timeout}).Dial
	}

	return &Client{
		proto:     proto,
		addr:      addr,
		scheme:    scheme,
		transport: tran,
	}
}
