package driverloader

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/vbox"
)

func Probe(driver string) (hypervisor.HypervisorDriver, error) {
	switch strings.ToLower(driver) {
	case "vbox", "":
		vd := vbox.InitDriver()
		if vd != nil {
			fmt.Printf("Vbox Driver Loaded.\n")
			return vd, nil
		}
	default:
		return nil, fmt.Errorf("Unsupported driver %s\n", driver)
	}

	return nil, fmt.Errorf("Driver %s is unavailable\n", driver)
}
