package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"

	gflag "github.com/jessevdk/go-flags"
)

// CmdPush pushes an image or repository to the registry.
//
// Usage: hyper push NAME[:TAG]
func (cli *HyperClient) HyperCmdPush(args ...string) error {
	// we need to get the image name which will be used to create a container
	if len(args) == 0 {
		return fmt.Errorf("\"push\" requires a minimum of 1 argument, please provide the image name.")
	}
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "push NAME[:TAG]\n\nPush an image to a Docker registry server"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	name := args[1]

	ref, err := reference.ParseNamed(name)
	if err != nil {
		return err
	}

	var tag string
	switch x := ref.(type) {
	case reference.Canonical:
		return fmt.Errorf("cannot push a digest reference")
	case reference.NamedTagged:
		tag = x.Tag()
	}

	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return err
	}

	// Resolve the Auth config relevant for this server
	authConfig := registry.ResolveAuthConfig(cli.configFile.AuthConfigs, repoInfo.Index)
	// If we're not using a custom registry, we know the restrictions
	// applied to repository names and can warn the user in advance.
	// Custom repositories can have different rules, and we must also
	// allow pushing by image ID.
	if repoInfo.Official {
		username := authConfig.Username
		if username == "" {
			username = "<user>"
		}
		return fmt.Errorf("You cannot push a \"root\" repository. Please rename your repository to <user>/<repo> (ex: %s/%s)", username, ref.Name())
	}

	v := url.Values{}
	v.Set("tag", tag)
	v.Set("remote", repoInfo.String())

	_, _, err = cli.clientRequestAttemptLogin("POST", "/image/push?"+v.Encode(), nil, cli.out, repoInfo.Index, "push")
	return err
}
