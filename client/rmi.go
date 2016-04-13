package client

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor/types"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdRmi(args ...string) error {
	var opts struct {
		Noprune bool `long:"no-prune" default:"false" default-mask:"-" description:"Do not delete untagged parents"`
		Force   bool `short:"f" long:"force" default:"false" default-mask:"-" description:"Force removal of the image"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "rmi [OPTIONS] IMAGE [IMAGE...]\n\nRemove one or more images"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"rmi\" requires a minimum of 1 argument, please provide IMAGE ID.\n")
	}
	images := args
	for _, imageId := range images {
		remoteInfo, err := cli.client.RemoveImage(imageId, opts.Noprune, opts.Force)
		if err != nil {
			fmt.Fprintf(cli.err, err.Error()+"\n")
			continue
		}

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
