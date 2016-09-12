package network

import (
	"fmt"
	"os"

	"github.com/hyperhq/runv/hypervisor/pod"
)

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Allocate(vmId, requestedIP string, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Configure(vmId, requestedIP string, addrOnly bool,
	maps []pod.UserContainerPort, config pod.UserInterface) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

// Release an interface for a select ip
func Release(vmId, releasedIP string, maps []pod.UserContainerPort, file *os.File) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}
