// +build !with_xen490

package xenpv

import "github.com/hyperhq/runv/hypervisor"

func InitDriver() hypervisor.HypervisorDriver {
	return nil
}
