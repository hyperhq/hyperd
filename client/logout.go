package client

import (
	"fmt"
	"strings"

	"github.com/docker/docker/registry"

	gflag "github.com/jessevdk/go-flags"
)

// CmdLogout logs a user out from a Docker registry.
//
// If no server is specified, the user will be logged out from the registry's index server.
//
// Usage: hyperctl logout [SERVER]
func (cli *HyperClient) HyperCmdLogout(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "logout [SERVER]\n\nLog out from a Docker registry, if no server is\nspecified \"" + registry.IndexServer + "\" is the default."
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	serverAddress := registry.IndexServer
	if len(args) > 0 {
		serverAddress = args[0]
	}

	if _, ok := cli.configFile.AuthConfigs[serverAddress]; !ok {
		fmt.Fprintf(cli.out, "Not logged in to %s\n", serverAddress)
	} else {
		fmt.Fprintf(cli.out, "Remove login credentials for %s\n", serverAddress)
		delete(cli.configFile.AuthConfigs, serverAddress)

		if err := cli.configFile.Save(); err != nil {
			return fmt.Errorf("Failed to save docker config: %v", err)
		}
	}
	return nil
}
