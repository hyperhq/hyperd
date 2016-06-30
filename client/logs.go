package client

import (
	"fmt"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdLogs(args ...string) error {
	var opts struct {
		Follow bool   `short:"f" long:"follow" default:"false" default-mask:"-" description:"Follow log output"`
		Since  string `long:"since" value-name:"\"\"" description:"Show logs since timestamp"`
		Times  bool   `short:"t" long:"timestamps" default:"false" default-mask:"-" description:"Show timestamps"`
		Tail   string `long:"tail" value-name:"\"all\"" description:"Number of lines to show from the end of the logs"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "logs CONTAINER [OPTIONS...]\n\nFetch the logs of a container"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("%s ERROR: Can not accept the 'logs' command without argument!\n", os.Args[0])
	}

	c, err := cli.client.GetContainerInfo(args[0])
	if err != nil {
		return err
	}

	output, ctype, err := cli.client.ContainerLogs(args[0], opts.Since, opts.Times, opts.Follow, true, true, opts.Tail)
	if err != nil {
		return err
	}
	return cli.readStreamOutput(output, ctype, c.Container.Tty, cli.out, cli.err)
}
