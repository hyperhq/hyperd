package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdRm(args ...string) error {
	var opts struct {
		Container bool `short:"c" long:"container" default:"false" default-mask:"-" description:"rm container"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "rm POD [POD...] | CONTAINER [CONTAINER...]\n\nRemove one or more pods | containers"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"rm\" requires a minimum of 1 argument, please provide POD | CONTAINER ID.\n")
	}

	if opts.Container {
		containers := args
		for _, id := range containers {
			err := cli.client.RemoveContainer(id)
			if err == nil {
				fmt.Fprintf(cli.out, "Container(%s) is successful to be deleted!\n", id)
			} else {
				fmt.Fprintf(cli.out, "%v\n", err)
			}
		}
	} else {
		pods := args
		for _, id := range pods {
			err := cli.client.RmPod(id)
			if err == nil {
				fmt.Fprintf(cli.out, "Pod(%s) is successful to be deleted!\n", id)
			} else {
				fmt.Fprintf(cli.out, "%v\n", err)
			}
		}
	}
	return nil
}
