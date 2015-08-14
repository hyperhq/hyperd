package client

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdPull(args ...string) error {
	// we need to get the image name which will be used to create a container
	if len(args) == 0 {
		return fmt.Errorf("\"pull\" requires a minimum of 1 argument, please provide the image name.")
	}
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "pull IMAGE\n\npull an image from a Docker registry server"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	imageName := args[1]
	v := url.Values{}
	v.Set("imageName", imageName)
	err = cli.stream("POST", "/image/create?"+v.Encode(), nil, os.Stdout, nil)
	if err != nil {
		return err
	}

	return nil
}
