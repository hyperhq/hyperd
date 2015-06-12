package client

import (
	"fmt"
	"net/url"
	"strings"

	gflag "github.com/jessevdk/go-flags"
	"hyper/engine"
	"hyper/types"
)

func (cli *HyperClient) HyperCmdRm(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "rm POD_ID\n\ndestroy a pod"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	if len(args) < 2 {
		return fmt.Errorf("\"rm\" requires a minimum of 1 argument, please provide POD ID.\n")
	}
	podId := args[1]
	v := url.Values{}
	v.Set("podId", podId)
	body, _, err := readBody(cli.call("POST", "/pod/remove?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		return fmt.Errorf("Error reading remote info: %s", err)
	}
	out.Close()
	errCode := remoteInfo.GetInt("Code")
	if errCode == types.E_OK || errCode == types.E_VM_SHUTDOWN {
		//fmt.Println("VM is successful to start!")
	} else {
		return fmt.Errorf("Error code is %d, Cause is %s", remoteInfo.GetInt("Code"), remoteInfo.Get("Cause"))
	}
	fmt.Printf("Pod(%s) is successful to be deleted!\n", podId)
	return nil
}
