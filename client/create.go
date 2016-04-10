package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor/pod"

	gflag "github.com/jessevdk/go-flags"
	"net/http"
)

func (cli *HyperClient) HyperCmdCreate(args ...string) error {
	var opts struct {
		Yaml bool `short:"y" long:"yaml" default:"false" default-mask:"-" description:"create a pod based on Yaml file"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "create [OPTIONS] POD_FILE\n\nCreate a pod into 'pending' status, but without running it"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("\"create\" requires a minimum of 1 argument, please provide POD spec file.\n")
	}
	jsonFile := args[0]

	jsonbody, err := cli.JsonFromFile(jsonFile, opts.Yaml, false)
	if err != nil {
		return err
	}

	var tmpPod pod.UserPod
	if err := json.Unmarshal([]byte(jsonbody), &tmpPod); err != nil {
		return err
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
	return nil
}
