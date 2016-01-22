package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/engine"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdPause(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "pause Pod\n\nPause the pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("Can not accept the 'pause' command without Pod ID!")
	}

	v := url.Values{}
	v.Set("podId", args[0])

	body, _, err := readBody(cli.call("POST", "/pod/pause?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}

	out := engine.NewOutput()
	if _, err = out.AddEnv(); err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return err
	}
	out.Close()
	return nil
}
