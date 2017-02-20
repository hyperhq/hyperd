package driverloader

import (
	"fmt"
	"os"
	"strings"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/libvirt"
	"github.com/hyperhq/runv/hypervisor/qemu"
	"github.com/hyperhq/runv/hypervisor/xen"
	"github.com/hyperhq/runv/lib/vsock"
)

func Probe(driver string) (hd hypervisor.HypervisorDriver, err error) {
	defer func() {
		if hd != nil && hypervisor.VsockCidManager == nil {
			hypervisor.VsockCidManager = vsock.NewDefaultVsockCidAllocator()
		}
	}()
	switch strings.ToLower(driver) {
	case "libvirt":
		ld := libvirt.InitDriver()
		if ld != nil {
			fmt.Printf("Libvirt Driver Loaded.\n")
			return ld, nil
		}
	case "kvm", "qemu-kvm":
		if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
			return nil, fmt.Errorf("Driver %s is unavailable\n", driver)
		}
		qd := qemu.InitDriver()
		if qd != nil {
			fmt.Printf("%s Driver Loaded\n", driver)
			return qd, nil
		}
	case "xen", "":
		xd := xen.InitDriver()
		if xd != nil {
			fmt.Printf("Xen Driver Loaded.\n")
			return xd, nil
		}
		if driver == "xen" {
			return nil, fmt.Errorf("Driver %s is unavailable\n", driver)
		}
		fallthrough // only for ""
	case "qemu": // "qemu" or "", kvm will be enabled if the system enables kvm
		qd := qemu.InitDriver()
		if qd != nil {
			fmt.Printf("Qemu Driver Loaded\n")
			return qd, nil
		}
	default:
		return nil, fmt.Errorf("Unsupported driver %s\n", driver)
	}

	return nil, fmt.Errorf("Driver %s is unavailable\n", driver)
}
