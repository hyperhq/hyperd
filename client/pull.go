package client

import (
	"fmt"
	"io"
	"strings"

	"github.com/hyperhq/runv/hypervisor/pod"

	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdPull(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "pull IMAGE\n\npull an image from a Docker registry server"
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
		return fmt.Errorf("\"pull\" requires a minimum of 1 argument, please provide the image name.")
	}
	return cli.PullImage(args[0])
}

func (cli *HyperClient) PullImage(imageName string) error {
	distributionRef, err := reference.ParseNamed(imageName)
	if err != nil {
		return err
	}
	if reference.IsNameOnly(distributionRef) {
		distributionRef = reference.WithDefaultTag(distributionRef)
		fmt.Fprintf(cli.out, "Using default tag: %s\n", reference.DefaultTag)
	}

	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := registry.ParseRepositoryInfo(distributionRef)
	if err != nil {
		return err
	}

	pull := func(auth types.AuthConfig) (io.ReadCloser, string, int, error) {
		return cli.client.Pull(distributionRef.String(), auth)
	}

	body, ctype, _, err := cli.requestWithLogin(repoInfo.Index, pull, "pull")
	if err != nil {
		return err
	}

	return cli.readStreamOutput(body, ctype, cli.isTerminalOut, cli.out, cli.err)
}

func (cli *HyperClient) PullImages(spec *pod.UserPod) error {
	for i := range spec.Containers {
		if err := cli.PullImage(spec.Containers[i].Image); err != nil {
			return err
		}
	}
	/* Hack here, pull service discovery image `haproxy` */
	if len(spec.Services) > 0 {
		return cli.PullImage("haproxy")
	}
	return nil
}
