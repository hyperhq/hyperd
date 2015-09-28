package client

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/runv/hypervisor/pod"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdPull(args ...string) error {
	// we need to get the image name which will be used to create a container
	if len(args) == 0 {
		return fmt.Errorf("\"pull\" requires a minimum of 1 argument, please provide the image name.")
	}
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "pull IMAGE\n\npull an image from a Docker registry server"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	return cli.PullImage(args[1])
}

func (cli *HyperClient) PullImage(imageName string) error {
	remote, _ := parsers.ParseRepositoryTag(imageName)
	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := registry.ParseRepositoryInfo(remote)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Set("imageName", imageName)
	_, _, err = cli.clientRequestAttemptLogin("POST", "/image/create?"+v.Encode(), nil, cli.out, repoInfo.Index, "pull")
	return err
}

func (cli *HyperClient) PullImages(data string) error {
	userpod, err := pod.ProcessPodBytes([]byte(data))
	if err != nil {
		return err
	}
	for _, c := range userpod.Containers {
		if err = cli.PullImage(c.Image); err != nil {
			return err
		}
	}
	/* Hack here, pull service discovery image `haproxy` */
	if len(userpod.Services) > 0 {
		return cli.PullImage("haproxy")
	}
	return nil
}
