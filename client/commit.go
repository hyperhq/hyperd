package client

import (
	"fmt"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

/*
	-a, --author=       Author (e.g., "Hello World <hello@a-team.com>")
	-c, --change=[]     Apply Dockerfile instruction to the created image
	-m, --message=      Commit message
	-p, --pause         Pause container during Commit
	-h, --help          Print usage
*/
func (cli *HyperClient) HyperCmdCommit(args ...string) error {
	var opts struct {
		Author  string   `short:"a" long:"author" default:"" value-name:"\"\"" description:"Author (e.g., \"Hello World <hello@a-team.com>\")"`
		Change  []string `short:"c" long:"change" default:"" value-name:"[]" description:"Apply Dockerfile instruction to the created image"`
		Message string   `short:"m" long:"message" default:"" value-name:"\"\"" description:"Commit message"`
		Pause   bool     `short:"p" long:"pause" default:"false" description:"Pause container during Commit"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)

	parser.Usage = "commit [OPTIONS] CONTAINER [REPOSITORY[:TAG]]\n\nCreate a new image from a container's changes"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("%s: \"commit\" requires a minimum of 1 argument, See 'hyperctl build --help'.", os.Args[0])
	}
	var (
		containerId string = ""
		repo        string = ""
	)
	if len(args) > 0 {
		containerId = args[0]
	}
	if len(args) > 1 {
		repo = args[1]
	}

	id, err := cli.client.Commit(containerId, repo, opts.Author, opts.Message, opts.Change, opts.Pause)
	if err != nil {
		return err
	}

	fmt.Fprintf(cli.out, "%s\n", id)
	return nil
}
