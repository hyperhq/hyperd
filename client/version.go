package client

import (
	"fmt"
	"os"

	"github.com/hyperhq/hyper/utils"
)

func (cli *HyperClient) HyperCmdVersion(args ...string) error {
	fmt.Fprintf(cli.out, "The %s version is %s\n", os.Args[0], utils.VERSION)
	return nil
}
