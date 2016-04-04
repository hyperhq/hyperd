package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/hyperhq/hyper/engine"

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
	v := url.Values{}
	v.Set("author", opts.Author)
	changeJson, err := json.Marshal(opts.Change)
	if err != nil {
		return err
	}
	v.Set("change", string(changeJson))
	v.Set("message", opts.Message)
	if opts.Pause == true {
		v.Set("pause", "yes")
	} else {
		v.Set("pause", "no")
	}
	v.Set("container", containerId)
	v.Set("repo", repo)
	body, _, err := readBody(cli.call("POST", "/container/commit?"+v.Encode(), nil, nil))
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
	fmt.Fprintf(cli.out, "%s\n", remoteInfo.Get("ID"))
	return nil
}
