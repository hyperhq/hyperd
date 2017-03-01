package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdSave(args ...string) error {
	var opts struct {
		Output string `short:"o" long:"output" value-name:"\"\"" description:"Write to a file, instead of STDOUT"`
	}

	output := cli.out
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "save [OPTIONS] IMAGE [IMAGE...]\n\nSave one or more images to a tar archive (streamed to STDOUT by default)"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("\"save\" requires a minimum of 1 argument, please provide IMAGE ID.\n")
	}

	if opts.Output == "" && cli.isTerminalOut {
		return errors.New("Cowardly refusing to save to a terminal. Use the -o flag or redirect.")
	}
	if opts.Output != "" {
		if output, err = os.Create(opts.Output); err != nil {
			return err
		}
	}

	images := args

	responseBody, err := cli.client.Save(images)
	if err != nil {
		fmt.Fprintf(cli.err, err.Error()+"\n")
		return err
	}
	defer responseBody.Close()
	_, err = io.Copy(output, responseBody)
	return err
}
