// +build linux,with_xen

package xen

import (
	"os"

	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (xd *XenDriver) BuildinNetwork() bool {
	return false
}

func (xd *XenDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (xc *XenContext) ConfigureNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort, config pod.UserInterface) (*network.Settings, error) {
	return nil, nil
}

func (xc *XenContext) AllocateNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, nil
}

func (xc *XenContext) ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort,
	file *os.File) error {
	return nil
}
