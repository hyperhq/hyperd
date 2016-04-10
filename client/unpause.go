package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdUnpause(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "unpause Pod\n\nUnpause the pod"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("Can not accept the 'unpause' command without Pod ID!")
	}

	return cli.client.UnpausePod(args[0])
}
