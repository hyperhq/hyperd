// +build !with_xen

package xen

import "github.com/hyperhq/runv/hypervisor"

func InitDriver() hypervisor.HypervisorDriver {
	return nil
}
