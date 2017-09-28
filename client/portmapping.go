package client

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/hyperhq/hyperd/types"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdPorts(args ...string) error {
	var opts struct {
		Portmap []string `short:"p" long:"publish" value-name:"[]" default-mask:"-" description:"Publish a container's port to the host, format: -p|--publish [tcp/udp:]hostPort:containerPort (only valid for add and delete)"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown|gflag.PassAfterNonOption)
	parser.Usage = "ports ls|add|delete [OPTIONS] POD\n\nList or modify port mapping rules of a Pod\n"

	if len(args) == 0 {
		parser.WriteHelp(cli.err)
		return nil
	}
	cmd := args[0]

	args, err := parser.ParseArgs(args[1:])
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	var modFunc func(string, []*types.PortMapping) error

	switch cmd {
	case "ls":
		if len(args) != 1 {
			return errors.New("need a Pod Id as command parameter")
		}
		pms, err := cli.client.ListPortMappings(args[0])
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cli.out, 20, 1, 3, ' ', 0)
		fmt.Fprintln(w, "Protocol\tHost Ports\tContainer Ports")
		for _, pm := range pms {
			fmt.Fprintf(w, "%s\t%s\t%s\n", pm.Protocol, pm.HostPort, pm.ContainerPort)
		}
		w.Flush()
		return nil
	case "add":
		modFunc = cli.client.AddPortMappings
	case "delete":
		modFunc = cli.client.DeletePortMappings
	default:
		parser.WriteHelp(cli.err)
		return nil
	}

	if len(args) != 1 {
		return errors.New("need a Pod Id as command parameter")
	}
	if len(opts.Portmap) == 0 {
		return errors.New("no rules to be add or delete")
	}

	pms := make([]*types.PortMapping, 0, len(opts.Portmap))
	for _, o := range opts.Portmap {
		pm, err := parsePortMapping(o)
		if err != nil {
			return fmt.Errorf("failed to parse rule %s: %v", o, err)
		}
		pms = append(pms, pm)
	}

	err = modFunc(args[0], pms)
	if err != nil {
		return err
	}
	return nil
}
