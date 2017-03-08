package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdStart(args ...string) error {
	var opts struct {
		Container bool `short:"c" long:"container" default-mask:"-" description:"start container"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "start [OPTIONS] POD_ID|CONTAINER_ID\n\nLaunch a created pod or container"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"start\" requires a minimum of 1 argument, please provide POD ID.\n")
	}
	var (
		id = args[0]
	)

	if !opts.Container {
		err = cli.client.StartPod(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(cli.out, "Successfully started the Pod(%s)\n", id)
	} else {
		err = cli.client.StartContainer(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(cli.out, "Successfully started container %s\n", id)
	}
	return nil
}
