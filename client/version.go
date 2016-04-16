package client

import (
	"fmt"
	"os"

	"github.com/hyperhq/hyperd/utils"
)

func (cli *HyperClient) HyperCmdVersion(args ...string) error {
	fmt.Fprintf(cli.out, "The %s version is %s\n", os.Args[0], utils.VERSION)
	return nil
}
