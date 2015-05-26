package client

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"
	"hyper/lib/term"
)

type HyperClient struct {
	proto      string
	addr       string
	scheme	   string
	in         io.ReadCloser
	out        io.Writer
	err        io.Writer
	inFd uintptr
	outFd uintptr
	isTerminalIn bool
	isTerminalOut bool
	tlsConfig	*tls.Config
	transport     *http.Transport
}

var funcMap = template.FuncMap{
	"json": func(v interface{}) string {
		a, _ := json.Marshal(v)
		return string(a)
	},
}

func (cli *HyperClient) getMethod(args ...string) (func(...string) error, bool) {
	camelArgs := make([]string, len(args))
	for i, s := range args {
		if len(s) == 0 {
			return nil, false
		}
		camelArgs[i] = strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
	}
	methodName := "HyperCmd" + strings.Join(camelArgs, "")
	method := reflect.ValueOf(cli).MethodByName(methodName)
	if !method.IsValid() {
		return nil, false
	}
	return method.Interface().(func(...string) error), true
}

// Cmd executes the specified command.
func (cli *HyperClient) Cmd(args ...string) error {
	if len(args) > 1 {
		method, exists := cli.getMethod(args[:2]...)
		if exists {
			return method(args[2:]...)
		}
	}
	if len(args) > 0 {
		method, exists := cli.getMethod(args[0])
		if !exists {
			fmt.Printf("%s: '%s' is not a %s command. See '%s --help'.\n", os.Args[0], args[0], os.Args[0], os.Args[0])
			os.Exit(1)
		}
		return method(args[1:]...)
	}
	return cli.HyperCmdHelp()
}

func NewHyperClient(proto, addr string, tlsConfig *tls.Config) *HyperClient {
	var (
		inFd          uintptr
		outFd         uintptr
		isTerminalIn  = false
		isTerminalOut = false
		scheme        = "http"
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

	inFd, isTerminalIn = term.GetFdInfo(os.Stdin)
	outFd, isTerminalOut = term.GetFdInfo(os.Stdout)

	return &HyperClient{
		proto:         proto,
		addr:          addr,
		in:            os.Stdin,
		out:           os.Stdout,
		err:           os.Stdout,
		inFd:          inFd,
		outFd:         outFd,
		isTerminalIn:  isTerminalIn,
		isTerminalOut: isTerminalOut,
		scheme:        scheme,
		transport:     tran,
	}
}
