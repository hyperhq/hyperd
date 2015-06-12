package client

import (
	"fmt"
	"os"
)

func (cli *HyperClient) HyperCmdHelp(args ...string) error {
	var helpMessage = `Usage:
  %s [OPTIONS] COMMAND [ARGS...]

Command:
  run                    create a pod, and launch a new pod
  start                  launch a 'pending' pod
  stop                   stop a running pod, it will become 'pending'
  exec                   run a command in a container of a running pod
  create                 create a pod into 'pending' status, but without running it
  replace                replace a running pod with a new one, the old one become 'pending'
  rm                     destroy a pod
  attach                 attach to the tty of a specified container in a pod

  pull                   pull an image from a Docker registry server
  info                   display system-wide information
  list                   list all pods or containers

Help Options:
  -h, --help             Show this help message

Run '%s COMMAND --help' for more information on a command.
`
	fmt.Printf(helpMessage, os.Args[0], os.Args[0])
	return nil
}
