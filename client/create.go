package client

import (
	"fmt"
	"strings"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdCreate(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("\"create\" requires a minimum of 1 argument, please provide POD spec file.\n")
	}
	var opts struct {
		Yaml bool `short:"y" long:"yaml" default:"false" default-mask:"-" description:"create a pod based on Yaml file"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "create [OPTIONS] POD_FILE\n\nCreate a pod into 'pending' status, but without running it"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	jsonFile := args[1]

	jsonbody, err := cli.JsonFromFile(jsonFile, opts.Yaml, false)
	if err != nil {
		return err
	}

	podId, err := cli.CreatePod(jsonbody, false, false)
	if err != nil {
		return err
	}
	fmt.Printf("Pod ID is %s\n", podId)
	return nil
}
