package client

import (
	"io"
	"os"
	"strings"

	runconfigopts "github.com/docker/docker/runconfig/opts"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdLoad(args ...string) error {
	var opts struct {
		Input string   `short:"i" long:"input" value-name:"\"\"" description:"Read from a tar archive file, instead of STDIN"`
		Name  string   `short:"n" long:"name" value-name:"\"\"" description:"Name to use when loading OCI image layout tar archive"`
		Refs  []string `short:"r" long:"references" value-name:"\"\"" description:"References to use when loading an OCI image layout tar archive"`
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
	output, ctype, err := cli.client.Load(input, opts.Name, runconfigopts.ConvertKVStringsToMap(opts.Refs))
	if err != nil {
		return err
	}
	return cli.readStreamOutput(output, ctype, false, cli.out, cli.err)
}
