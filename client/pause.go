package client

import (
	"fmt"
	"strings"

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

	return cli.client.PausePod(args[0])
}
