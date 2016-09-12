package qemu

import (
	"os"

	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (qd *QemuDriver) BuildinNetwork() bool {
	return false
}

func (qd *QemuDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (qc *QemuContext) ConfigureNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort, config pod.UserInterface) (*network.Settings, error) {
	return nil, nil
}

func (qc *QemuContext) AllocateNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, nil
}

func (qc *QemuContext) ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort,
	file *os.File) error {
	return nil
}
