package client

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdList(args ...string) error {
	var opts struct {
		Aux bool   `short:"x" long:"aux" default:"false" description:"show the auxiliary containers"`
		Pod string `short:"p" long:"pod" value-name:"\"\"" description:"only list the specified pod"`
		VM  string `short:"m" long:"vm" value-name:"\"\"" description:"only list resources on the specified vm"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "list [OPTIONS] [pod|container|vm]\n\nlist all pods or container information"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	var item string
	if len(args) == 0 {
		item = "pod"
	} else {
		item = args[0]
	}

	if item != "pod" && item != "vm" && item != "container" {
		return fmt.Errorf("Error, the %s can not support %s list!", os.Args[0], item)
	}

	remoteInfo, err := cli.client.List(item, opts.Pod, opts.VM, opts.Aux)
	if err != nil {
		return err
	}

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

	w := tabwriter.NewWriter(cli.out, 20, 1, 3, ' ', 0)
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
		fmt.Fprintln(w, "VM name\tStatus")
		for _, vm := range vmResponse {
			fields := strings.Split(vm, ":")
			fmt.Fprintf(w, "%s\t%s\n", fields[0], fields[2])
		}
	}

	if item == "pod" {
		fmt.Fprintln(w, "POD ID\tPOD Name\tVM name\tStatus")
		for _, p := range podResponse {
			fields := strings.Split(p, ":")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fields[0], fields[1], fields[2], fields[3])
		}
	}

	if item == "container" {
		fmt.Fprintln(w, "Container ID\tName\tPOD ID\tStatus")
		for _, c := range containerResponse {
			fields := strings.Split(c, ":")
			name := fields[1]
			if len(name) > 0 {
				if name[0] == '/' {
					name = name[1:]
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fields[0], name, fields[2], fields[3])
		}
	}
	w.Flush()
	return nil
}
