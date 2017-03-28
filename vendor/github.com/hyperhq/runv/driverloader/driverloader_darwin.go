package driverloader

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/vbox"
)

func Probe(driver string) (hypervisor.HypervisorDriver, error) {
	switch strings.ToLower(driver) {
	case "vbox", "":
		vd := vbox.InitDriver()
		if vd != nil {
			glog.V(1).Infof("Driver \"vbox\" loaded")
			return vd, nil
		}
	default:
		return nil, fmt.Errorf("Unsupported driver %q", driver)
	}

	return nil, fmt.Errorf("Driver %q is unavailable", driver)
}
