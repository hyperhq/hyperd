// +build linux,with_xen

package xen

import (
	"os"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (xd *XenDriver) BuildinNetwork() bool {
	return false
}

func (xd *XenDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (xc *XenContext) ConfigureNetwork(vmId, requestedIP string, config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (xc *XenContext) AllocateNetwork(vmId, requestedIP string) (*network.Settings, error) {
	return nil, nil
}

func (xc *XenContext) ReleaseNetwork(vmId, releasedIP string, file *os.File) error {
	return nil
}
