package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/registry"

	gflag "github.com/jessevdk/go-flags"
)

// CmdPush pushes an image or repository to the registry.
//
// Usage: hyper push NAME[:TAG]
func (cli *HyperClient) HyperCmdPush(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "push NAME[:TAG]\n\nPush an image to a Docker registry server"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	// we need to get the image name which will be used to create a container
	if len(args) == 0 {
		return fmt.Errorf("\"push\" requires a minimum of 1 argument, please provide the image name.")
	}
	name := args[0]
	remote, tag := parsers.ParseRepositoryTag(name)

	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := registry.ParseRepositoryInfo(remote)
	if err != nil {
		return err
	}
	// Resolve the Auth config relevant for this server
	authConfig := registry.ResolveAuthConfig(cli.configFile, repoInfo.Index)
	// If we're not using a custom registry, we know the restrictions
	// applied to repository names and can warn the user in advance.
	// Custom repositories can have different rules, and we must also
	// allow pushing by image ID.
	if repoInfo.Official {
		username := authConfig.Username
		if username == "" {
			username = "<user>"
		}
		return fmt.Errorf("You cannot push a \"root\" repository. Please rename your repository to <user>/<repo> (ex: %s/%s)", username, repoInfo.LocalName)
	}

	v := url.Values{}
	v.Set("tag", tag)
	v.Set("remote", remote)

	_, _, err = cli.clientRequestAttemptLogin("POST", "/image/push?"+v.Encode(), nil, cli.out, repoInfo.Index, "push")
	return err
}
