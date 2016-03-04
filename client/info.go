package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/hyper/engine"

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
	body, _, err := readBody(cli.call("GET", "/info", nil, nil))
	if err != nil {
		return err
	}

	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return err
	}
	out.Close()
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
	var rtn float64
	if s < 1024*1024 {
		return fmt.Sprintf("%d KB", s)
	} else if s < 1024*1024*1024 {
		rtn = float64(s) / (1024.0 * 1024.0)
		return fmt.Sprintf("%.1f MB", rtn)
	} else {
		rtn = float64(s) / (1024.0 * 1024.0 * 1024.0)
		return fmt.Sprintf("%.1f GB", rtn)
	}
}
