package client

import (
	"io"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdLoad(args ...string) error {
	var opts struct {
		Input string `short:"i" long:"input" value-name:"\"\"" description:"Read from a tar archive file, instead of STDIN"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "load [OPTIONS]\n\nLoad a image from STDIN or tar archive file"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	var input io.Reader = cli.in
	if opts.Input != "" {
		file, err := os.Open(opts.Input)
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
	}
	output, ctype, err := cli.client.Load(input)
	if err != nil {
		return err
	}
	return cli.readStreamOutput(output, ctype, false, cli.out, cli.err)
}
