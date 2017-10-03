// +build !with_xen

package xenpv

import "github.com/hyperhq/runv/hypervisor"

func InitDriver() hypervisor.HypervisorDriver {
	return nil
}
