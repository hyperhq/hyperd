package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdRmi(args ...string) error {
	var opts struct {
		Noprune bool `long:"no-prune" default:"false" default-mask:"-" description:"Do not delete untagged parents"`
		Force   bool `short:"f" long:"force" default:"true" default-mask:"-" description:"Force removal of the image"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "rmi [OPTIONS] IMAGE [IMAGE...]\n\nRemove one or more images"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	var (
		noprune string = "no"
		force   string = "yes"
	)
	if len(args) < 2 {
		return fmt.Errorf("\"rmi\" requires a minimum of 1 argument, please provide IMAGE ID.\n")
	}
	if opts.Noprune == true {
		noprune = "yes"
	}
	if opts.Force == false {
		force = "no"
	}
	images := args[1:]
	for _, imageId := range images {
		v := url.Values{}
		v.Set("imageId", imageId)
		v.Set("noprune", noprune)
		v.Set("force", force)
		body, _, err := readBody(cli.call("POST", "/images/remove?"+v.Encode(), nil, nil))
		if err != nil {
			fmt.Fprintf(cli.err, "Error remove the image(%s): %s\n", imageId, err.Error())
			continue
		}
		out := engine.NewOutput()
		remoteInfo, err := out.AddEnv()
		if err != nil {
			fmt.Fprintf(cli.err, "Error remove the image(%s): %s\n", imageId, err.Error())
			continue
		}

		if _, err := out.Write(body); err != nil {
			fmt.Fprintf(cli.err, "Error remove the image(%s): %s\n", imageId, err.Error())
			continue
		}
		out.Close()
		errCode := remoteInfo.GetInt("Code")
		if errCode == types.E_OK || errCode == types.E_VM_SHUTDOWN {
			//fmt.Println("VM is successful to start!")
			fmt.Fprintf(cli.out, "Image(%s) is successful to be deleted!\n", imageId)
		} else {
			fmt.Fprintf(cli.err, "Error remove the image(%s): %s\n", imageId, remoteInfo.Get("Cause"))
		}
	}
	return nil
}
