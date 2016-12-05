package pod

import (
	"fmt"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	apitypes "github.com/hyperhq/hyperd/types"
	runv "github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

const DEFAULT_INTERFACE_NAME = "eth-default"

type Interface struct {
	p *XPod

	spec     *apitypes.UserInterface
	descript *runv.InterfaceDescription
}

func newInterface(p *XPod, spec *apitypes.UserInterface) *Interface {
	if spec.Ifname == "" {
		spec.Ifname = DEFAULT_INTERFACE_NAME
	}
	return &Interface{p: p, spec: spec}
}

func (inf *Interface) LogPrefix() string {
	return fmt.Sprintf("%sNic[%s] ", inf.p.LogPrefix(), inf.spec.Ifname)
}

func (inf *Interface) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, inf, 1, args...)
}

func (inf *Interface) prepare() error {

	defer inf.Log(DEBUG, "prepare inf info: %#v", inf.descript)

	if inf.spec.Ip == "" && inf.spec.Bridge != "" {
		err := fmt.Errorf("if configured a bridge, must specify the IP address")
		inf.Log(ERROR, err)
		return err
	}

	if inf.spec.Ip == "" {
		setting, err := network.AllocateAddr("")
		if err != nil {
			inf.Log(ERROR, "failed to allocate IP: %v", err)
			return err
		}
		inf.descript = &runv.InterfaceDescription{
			Id:     setting.IPAddress,
			Lo:     false,
			Bridge: setting.Bridge,
			Ip:     setting.IPAddress,
			Mac:    setting.Mac,
			Gw:     setting.Gateway,
		}
		return nil
	}

	inf.descript = &runv.InterfaceDescription{
		Id:      inf.spec.Ifname,
		Lo:      false,
		Bridge:  inf.spec.Bridge,
		Ip:      inf.spec.Ip,
		Mac:     inf.spec.Mac,
		Gw:      inf.spec.Gateway,
		TapName: inf.spec.Tap,
	}

	return nil
}

func (inf *Interface) add() error {
	if inf.descript == nil || inf.descript.Ip == "" {
		err := fmt.Errorf("interfice has not ready %#v", inf.descript)
		inf.Log(ERROR, err)
		return err
	}
	err := inf.p.sandbox.AddNic(inf.descript)
	if err != nil {
		inf.Log(ERROR, "failed to add NIC: %v", err)
	}
	return err
}

func (inf *Interface) cleanup() error {
	if inf.spec.Ip != "" || inf.descript == nil || inf.descript.Ip == "" {
		return nil
	}

	inf.Log(DEBUG, "release IP address: %s", inf.descript.Ip)
	err := network.ReleaseAddr(inf.descript.Ip)
	if err != nil {
		inf.Log(ERROR, "failed to release IP %s: %v", inf.descript.Ip, nil)
	}
	return err
}
