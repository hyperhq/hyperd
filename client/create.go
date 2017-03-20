package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	gflag "github.com/jessevdk/go-flags"

	apitype "github.com/hyperhq/hyperd/types"
)

func (cli *HyperClient) HyperCmdCreate(args ...string) error {
	var (
		parser *gflag.Parser
		opts   = &CreateFlags{}
		err    error
		podId  string
	)
	parser = gflag.NewParser(opts, gflag.Default|gflag.IgnoreUnknown|gflag.PassAfterNonOption)
	parser.Usage = "create [OPTIONS] [POD_ID] IMAGE [COMMAND] [ARG...]\n\nCreate a pod, or create a container in the pod specified by the POD_ID"
	args, err = parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if opts.Container {
		if len(args) == 0 {
			return fmt.Errorf("%s: \"create\" requires the pod ID as first argument.", os.Args[0])
		}
		podId = args[0]
		args = args[1:]
	}

	specjson, err := cli.ParseCommonOptions(&opts.CommonFlags, opts.Container, args...)
	if err != nil {
		return err
	}

	if !opts.Container {
		var tmpPod apitype.UserPod
		err := json.Unmarshal(specjson, &tmpPod)
		if err != nil {
			return fmt.Errorf("failed to read json: %v", err)
		}
		podId, statusCode, err := cli.client.CreatePod(&tmpPod)
		if err != nil {
			if statusCode == http.StatusNotFound {
				err = cli.PullImages(&tmpPod)
				if err != nil {
					return err
				}
				podId, statusCode, err = cli.client.CreatePod(&tmpPod)
			}
			if err != nil {
				return err
			}
		}
		fmt.Printf("Pod ID is %s\n", podId)
	} else {
		var tmpContainer apitype.UserContainer
		err := json.Unmarshal(specjson, &tmpContainer)
		if err != nil {
			return fmt.Errorf("failed to read json: %v", err)
		}
		cid, statusCode, err := cli.client.CreateContainer(podId, &tmpContainer)
		if err != nil {
			if statusCode == http.StatusNotFound {
				err = cli.PullImage(tmpContainer.Image)
				if err != nil {
					return err
				}
				cid, statusCode, err = cli.client.CreateContainer(podId, &tmpContainer)
			}
			if err != nil {
				return err
			}
		}
		fmt.Printf("Container ID is %s\n", cid)
	}

	return nil
}
