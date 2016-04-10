package client

import (
	"fmt"
	"os"
)

func (cli *HyperClient) HyperCmdHelp(args ...string) error {
	var helpMessage = `Usage:
  %s [OPTIONS] COMMAND [ARGS...]

Command:
  attach                 Attach to the tty of a specified container in a pod
  build                  Build an image from a Dockerfile
  commit                 Create a new image from a container's changes
  create                 Create a pod into 'pending' status, but without running it
  exec                   Run a command in a container of a running pod
  images                 List images
  info                   Display system-wide information
  list                   List all pods or containers
  load                   Load a image from STDIN or tar archive file
  login                  Register or log in to a Docker registry server
  logout                 Log out from a Docker registry server
  pull                   Pull an image from a Docker registry server
  push                   Push an image or a repository to a Docker registry server
  rm                     Remove one or more pods
  rmi                    Remove one or more images
  run                    Create a pod, and launch a new pod
  start                  Launch a 'pending' pod
  stop                   Stop a running pod, it will become 'pending'

Help Options:
  -h, --help             Show this help message

Run '%s COMMAND --help' for more information on a command.
`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
	return nil
}
