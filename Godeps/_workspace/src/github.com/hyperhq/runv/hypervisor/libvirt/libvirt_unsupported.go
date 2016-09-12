// +build !with_libvirt

package libvirt

import "github.com/hyperhq/runv/hypervisor"

func InitDriver() hypervisor.HypervisorDriver {
	return nil
}
