// +build !linux,!darwin

package network

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func Allocate(vmId, requestedIP string, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	return nil, nil
}

func Configure(vmId, requestedIP string, addrOnly bool,
	maps []pod.UserContainerPort, config pod.UserInterface) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Release(vmId, releasedIP string, maps []pod.UserContainerPort, file *os.File) error {
	return nil
}
