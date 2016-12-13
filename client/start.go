package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdStart(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "start POD_ID [CONTAINER_ID]\n\nLaunch a created pod or container"
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
		podId       = args[0]
		containerId string
	)
	if len(args) >= 2 {
		containerId = args[1]
	}

	if containerId == "" {
		_, err = cli.client.StartPod(podId, "", false, false, nil, nil, nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(cli.out, "Successfully started the Pod(%s)\n", podId)
	} else {
		err = cli.client.StartContainer(podId, containerId)
		if err != nil {
			return err
		}
		fmt.Fprintf(cli.out, "Successfully started container %s in pod %s\n", containerId, podId)
	}
	return nil
}
