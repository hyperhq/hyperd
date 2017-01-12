package qemu

import (
	"os"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (qd *QemuDriver) BuildinNetwork() bool {
	return false
}

func (qd *QemuDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (qc *QemuContext) ConfigureNetwork(vmId, requestedIP string, config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (qc *QemuContext) AllocateNetwork(vmId, requestedIP string) (*network.Settings, error) {
	return nil, nil
}

func (qc *QemuContext) ReleaseNetwork(vmId, releasedIP string, file *os.File) error {
	return nil
}
