package client

import (
	"fmt"
	"strings"

	"github.com/docker/go-units"

	gflag "github.com/jessevdk/go-flags"
)

// we need this *info* function to get the whole status from the hyper daemon
func (cli *HyperClient) HyperCmdInfo(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "info\n\nDisplay system-wide information"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	remoteInfo, err := cli.client.Info()
	if err != nil {
		return err
	}

	fmt.Fprintf(cli.out, "Images: %d\n", remoteInfo.GetInt("Images"))
	if remoteInfo.Exists("Containers") {
		fmt.Fprintf(cli.out, "Containers: %d\n", remoteInfo.GetInt("Containers"))
	}
	fmt.Fprintf(cli.out, "PODs: %d\n", remoteInfo.GetInt("Pods"))
	fmt.Fprintf(cli.out, "Storage Driver: %s\n", remoteInfo.Get("Driver"))
	var status [][]string
	err = remoteInfo.GetJson("DriverStatus", &status)
	if err == nil {
		for _, pair := range status {
			fmt.Fprintf(cli.out, "  %s: %s\n", pair[0], pair[1])
		}
	}

	fmt.Fprintf(cli.out, "Hyper Root Dir: %s\n", remoteInfo.Get("DockerRootDir"))
	fmt.Fprintf(cli.out, "Index Server Address: %s\n", remoteInfo.Get("IndexServerAddress"))
	fmt.Fprintf(cli.out, "Execution Driver: %s\n", remoteInfo.Get("ExecutionDriver"))

	memTotal := getMemSizeString(remoteInfo.GetInt("MemTotal"))
	fmt.Fprintf(cli.out, "Total Memory: %s\n", memTotal)
	fmt.Fprintf(cli.out, "Operating System: %s\n", remoteInfo.Get("Operating System"))

	return nil
}

func getMemSizeString(s int) string {
	rtn := float64(s)
	return units.HumanSize(rtn)
}
