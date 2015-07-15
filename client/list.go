package client

import (
	"fmt"
	"github.com/hyperhq/hyper/engine"
	gflag "github.com/jessevdk/go-flags"
	"net/url"
	"os"
	"strings"
)

func (cli *HyperClient) HyperCmdList(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "list [pod|container]\n\nlist all pods or container information"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	var item string
	if len(args) == 1 {
		item = "pod"
	} else {
		item = args[1]
	}

	if item != "pod" && item != "vm" && item != "container" {
		return fmt.Errorf("Error, the %s can not support %s list!", os.Args[0], item)
	}

	v := url.Values{}
	v.Set("item", item)
	body, _, err := readBody(cli.call("GET", "/list?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return err
	}
	out.Close()

	var (
		vmResponse        = []string{}
		podResponse       = []string{}
		containerResponse = []string{}
	)
	if remoteInfo.Exists("item") {
		item = remoteInfo.Get("item")
	}
	if remoteInfo.Exists("Error") {
		return fmt.Errorf("Found an error while getting %s list: %s", item, remoteInfo.Get("Error"))
	}

	if item == "vm" {
		vmResponse = remoteInfo.GetList("vmData")
	}
	if item == "pod" {
		podResponse = remoteInfo.GetList("podData")
	}
	if item == "container" {
		containerResponse = remoteInfo.GetList("cData")
	}

	//fmt.Printf("Item is %s\n", item)
	if item == "vm" {
		fmt.Printf("%15s%20s\n", "VM name", "Status")
		for _, vm := range vmResponse {
			fields := strings.Split(vm, ":")
			fmt.Printf("%15s%20s\n", fields[0], fields[2])
		}
	}

	if item == "pod" {
		fmt.Printf("%15s%30s%20s%10s\n", "POD ID", "POD Name", "VM name", "Status")
		for _, p := range podResponse {
			fields := strings.Split(p, ":")
			var podName = fields[1]
			if len(fields[1]) > 27 {
				podName = fields[1][:27]
			}
			fmt.Printf("%15s%30s%20s%10s\n", fields[0], podName, fields[2], fields[3])
		}
	}

	if item == "container" {
		fmt.Printf("%-66s%15s%10s\n", "Container ID", "POD ID", "Status")
		for _, c := range containerResponse {
			fields := strings.Split(c, ":")
			fmt.Printf("%-66s%15s%10s\n", fields[0], fields[1], fields[2])
		}
	}
	return nil
}
