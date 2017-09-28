package client

import (
	"fmt"
	"os"
)

func (cli *HyperClient) HyperCmdHelp(args ...string) error {
	var helpMessage = `Usage:
  %s [OPTIONS] COMMAND [ARGS...]

Command:
  attach                 Attach to the input/output of a specified container
  build                  Build an image from a Dockerfile
  commit                 Create a new image from a container's changes
  create                 Create a pod or create a container in a pod
  exec                   Run a command in a specified container
  images                 List images
  info                   Display system-wide information
  list                   List all pods or containers
  load                   Load a image from STDIN or tar archive file
  login                  Register or log in to a Docker registry server
  logout                 Log out from a Docker registry server
  pause                  Pause a running pod
  ports                  Show or modify port mapping rules
  pull                   Pull an image from a Docker registry server
  push                   Push an image or a repository to a Docker registry server
  rm                     Remove one or more pods or containers
  rmi                    Remove one or more images
  run                    Create a pod, and launch the new pod
  save                   Save one or more images to a tar archive (streamed to STDOUT by default)
  start                  Start a pod or container
  stop                   Stop a running pod or container
  unpause                Unpause a paused pod

Help Options:
  -h, --help             Show this help message

Run '%s COMMAND --help' for more information on a command.
`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
	return nil
}
