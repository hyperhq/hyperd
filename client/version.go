package client

import (
	"fmt"
	"github.com/hyperhq/hyper/utils"
	"os"
)

func (cli *HyperClient) HyperCmdVersion(args ...string) error {
	fmt.Printf("The %s version is %s\n", os.Args[0], utils.VERSION)
	return nil
}
