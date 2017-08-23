// +build !linux,!darwin

package network

import (
	"fmt"
	"os"

	"github.com/hyperhq/runv/api"
)

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func AllocateAddr(requestedIP string) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Configure(inf *api.InterfaceDescription) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func ReleaseAddr(releasedIP string) error {
	return nil
}
