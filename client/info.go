package client

import (
	"fmt"
	"strings"
	"hyper/engine"
	gflag "github.com/jessevdk/go-flags"
)

// we need this *info* function to get the whole status from the docker daemon
func (cli *HyperClient) HyperCmdInfo(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "info\n\ndisplay system-wide information"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	body, _, err := readBody(cli.call("GET", "/info", nil, nil))
	if err != nil {
		fmt.Printf("An error is encountered, %s\n", err)
		return err
	}

	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return err
	}
	out.Close()
	if remoteInfo.Exists("Containers") {
		fmt.Printf("Containers: %d\n", remoteInfo.GetInt("Containers"))
	}
	fmt.Printf("PODs: %d\n", remoteInfo.GetInt("Pods"))
	memTotal := remoteInfo.GetInt("MemTotal")
	fmt.Printf("Total Memory: %d KB\n", memTotal)
	fmt.Printf("Operating System: %s\n", remoteInfo.Get("Operating System"))

	return nil
}
