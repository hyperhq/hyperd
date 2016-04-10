package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdRm(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "rm POD [POD...]\n\nRemove one or more pods"
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
	pods := args
	for _, id := range pods {
		err := cli.client.RmPod(id)
		if err == nil {
			fmt.Fprintf(cli.out, "Pod(%s) is successful to be deleted!\n", id)
		} else {
			fmt.Fprintf(cli.out, "%v\n", err)
		}
	}
	return nil
}
