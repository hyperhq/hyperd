package network

import (
	"fmt"
	"os"

	"github.com/hyperhq/runv/api"
)

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Allocate(vmId, requestedIP string, addrOnly bool, maps []*api.PortDescription) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Configure(vmId, requestedIP string, addrOnly bool,
	maps []*api.PortDescription, inf *api.InterfaceDescription) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

// Release an interface for a select ip
func Release(vmId, releasedIP string, maps []*api.PortDescription, file *os.File) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}
