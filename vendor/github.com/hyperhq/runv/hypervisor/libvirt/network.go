// +build linux,with_libvirt

package libvirt

import (
	"os"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (ld *LibvirtDriver) BuildinNetwork() bool {
	return true
}

func (ld *LibvirtDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return network.InitNetwork(bIface, bIP, disableIptables)
}

func (lc *LibvirtContext) ConfigureNetwork(vmId, requestedIP string, config *api.InterfaceDescription) (*network.Settings, error) {
	return network.Configure(vmId, requestedIP, true, config)
}

func (lc *LibvirtContext) AllocateNetwork(vmId, requestedIP string) (*network.Settings, error) {
	return network.Allocate(vmId, requestedIP, true)
}

func (lc *LibvirtContext) ReleaseNetwork(vmId, releasedIP string, file *os.File) error {
	return network.Release(vmId, releasedIP)
}
