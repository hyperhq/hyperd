package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	apitype "github.com/hyperhq/hyperd/types"
)

func (cli *HyperClient) HyperCmdCreate(args ...string) error {
	copt, err := cli.ParseCreateOptions("create", args...)
	if err != nil {
		return err
	}
	if copt.Remove || copt.Attach {
		return fmt.Errorf("\"create\" does not support attach and rm parameter")
	}

	if copt.IsContainer && copt.PodId == "" {
		return fmt.Errorf("did not provide the target pod")
	}

	if !copt.IsContainer {
		var tmpPod apitype.UserPod
		err := json.Unmarshal(copt.JsonBytes, &tmpPod)
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
		err := json.Unmarshal(copt.JsonBytes, &tmpContainer)
		if err != nil {
			return fmt.Errorf("failed to read json: %v", err)
		}
		cid, statusCode, err := cli.client.CreateContainer(copt.PodId, &tmpContainer)
		if err != nil {
			if statusCode == http.StatusNotFound {
				err = cli.PullImage(tmpContainer.Image)
				if err != nil {
					return err
				}
				cid, statusCode, err = cli.client.CreateContainer(copt.PodId, &tmpContainer)
			}
			if err != nil {
				return err
			}
		}
		fmt.Printf("Container ID is %s\n", cid)
	}

	return nil
}
