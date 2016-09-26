package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	"github.com/hyperhq/hyperd/client/api"
	"github.com/hyperhq/runv/lib/term"

	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/pkg/homedir"
)

type HyperClient struct {
	client        api.APIInterface
	in            io.ReadCloser
	out           io.Writer
	err           io.Writer
	inFd          uintptr
	outFd         uintptr
	isTerminalIn  bool
	isTerminalOut bool
	configFile    *cliconfig.ConfigFile
}

func NewHyperClient(proto, addr string, tlsConfig *tls.Config) *HyperClient {
	var (
		inFd          uintptr
		outFd         uintptr
		isTerminalIn  = false
		isTerminalOut = false
	)

	clifile, err := cliconfig.Load(filepath.Join(homedir.Get(), ".docker"))
	if err != nil {
		fmt.Fprintf(os.Stdout, "WARNING: Error loading config file %v\n", err)
	}

	inFd, isTerminalIn = term.GetFdInfo(os.Stdin)
	outFd, isTerminalOut = term.GetFdInfo(os.Stdout)

	return &HyperClient{
		client:        api.NewClient(proto, addr, tlsConfig),
		in:            os.Stdin,
		out:           os.Stdout,
		err:           os.Stderr,
		inFd:          inFd,
		outFd:         outFd,
		isTerminalIn:  isTerminalIn,
		isTerminalOut: isTerminalOut,
		configFile:    clifile,
	}
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
