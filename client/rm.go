package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdRm(args ...string) error {
	var opts struct {
		Container bool `short:"c" long:"container" default-mask:"-" description:"stop container"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "rm [OPTIONS] CONTAINER|POD [CONTAINER|POD...]\n\nRemove one or more containers/pods"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"rm\" requires a minimum of 1 argument, please provide POD ID.\n")
	}
	for _, id := range args {
		if opts.Container {
			err := cli.client.RemoveContainer(id)
			if err == nil {
				fmt.Fprintf(cli.out, "container %s is successfully deleted!\n", id)
			} else {
				fmt.Fprintf(cli.err, "container %s delete failed: %v\n", id, err)
			}
		} else {
			err := cli.client.RmPod(id)
			if err == nil {
				fmt.Fprintf(cli.out, "Pod(%s) is successfully deleted!\n", id)
			} else {
				fmt.Fprintf(cli.err, "Pod(%s) delete failed: %v\n", id, err)
			}
		}
	}
	return nil
}
