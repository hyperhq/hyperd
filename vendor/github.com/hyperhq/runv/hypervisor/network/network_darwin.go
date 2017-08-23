package network

import (
	"fmt"

	"github.com/hyperhq/runv/api"
)

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Configure(inf *api.InterfaceDescription) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func AllocateAddr(requestedIP string) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

// Release an interface for a select ip
func ReleaseAddr(releasedIP string) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}
