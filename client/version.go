package client

import (
	"fmt"
	"os"
	"hyper/utils"
)

func (cli *HyperClient) HyperCmdVersion(args ...string) error {
	fmt.Printf("The %s version is %s\n", os.Args[0], utils.VERSION)
	return nil
}
