package pod

import (
	"fmt"
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/networking/portmapping"
	apitypes "github.com/hyperhq/hyperd/types"
)

func translatePortMapping(spec []*apitypes.PortMapping) ([]*portmapping.PortMapping, error) {
	if len(spec) == 0 {
		return []*portmapping.PortMapping{}, nil
	}
	var pms = make([]*portmapping.PortMapping, 0, len(spec))
	for _, entry := range spec {
		pm, err := portmapping.NewPortMapping(entry.Protocol, entry.HostPort, entry.ContainerPort)
		if err != nil {
			hlog.Log(ERROR, "failed to parsing portmappings: %v", err)
			return pms, err
		}
		hlog.Log(TRACE, "parsed portmapping rule %#v", entry)
		pms = append(pms, pm)
	}
	return pms, nil
}

func (p *XPod) initPortMapping() error {
	if p.containerIP != "" && len(p.portMappings) > 0 {
		pms, err := translatePortMapping(p.portMappings)
		if err != nil {
			hlog.Log(ERROR, err)
			return err
		}
		var extPrefix []string
		if p.globalSpec.PortmappingWhiteLists != nil &&
			len(p.globalSpec.PortmappingWhiteLists.InternalNetworks) > 0 &&
			len(p.globalSpec.PortmappingWhiteLists.ExternalNetworks) > 0 {
			extPrefix = p.globalSpec.PortmappingWhiteLists.ExternalNetworks
		}
		preExec, err := portmapping.SetupPortMaps(p.containerIP, extPrefix, pms)
		if err != nil {
			p.Log(ERROR, "failed to setup port mappings: %v", err)
			return err
		}
		if len(preExec) > 0 {
			p.prestartExecs = append(p.prestartExecs, preExec...)
		}
	}
	return nil
}

func (p *XPod) flushPortMapping() error {
	if p.containerIP != "" && len(p.portMappings) > 0 {
		pms, err := translatePortMapping(p.portMappings)
		if err != nil {
			hlog.Log(ERROR, err)
			return err
		}
		_, err = portmapping.ReleasePortMaps(p.containerIP, nil, pms)
		if err != nil {
			p.Log(ERROR, "release port mappings failed: %v", err)
			return err
		}
	}
	return nil
}

func (p *XPod) AddPortMapping(spec []*apitypes.PortMapping) error {
	if !p.IsAlive() {
		err := fmt.Errorf("portmapping could apply to running pod only (%v)", spec)
		p.Log(ERROR, "port mapping failed: %v", err)
		return err
	}
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if p.containerIP == "" || len(spec) == 0 {
		p.Log(INFO, "Skip port maping setup [%v], container IP: %s", spec, p.containerIP)
		return nil
	}

	pms, err := translatePortMapping(spec)
	if err != nil {
		p.Log(ERROR, "failed to generate port mapping rules: %v", err)
		return err
	}
	var extPrefix []string
	if p.globalSpec.PortmappingWhiteLists != nil &&
		len(p.globalSpec.PortmappingWhiteLists.InternalNetworks) > 0 &&
		len(p.globalSpec.PortmappingWhiteLists.ExternalNetworks) > 0 {
		extPrefix = p.globalSpec.PortmappingWhiteLists.ExternalNetworks
	}
	preExec, err := portmapping.SetupPortMaps(p.containerIP, extPrefix, pms)
	if err != nil {
		p.Log(ERROR, "failed to apply port mapping rules: %v", err)
		return err
	}
	if len(preExec) > 0 {
		p.prestartExecs = append(p.prestartExecs, preExec...)
		if p.sandbox != nil {
			for _, ex := range preExec {
				_, stderr, err := p.sandbox.HyperstartExecSync(ex, nil)
				if err != nil {
					p.Log(ERROR, "failed to setup inSandbox mapping: %v [ %s", err, string(stderr))
					return err
				}
			}
		}
	}

	all := make([]*apitypes.PortMapping, len(p.portMappings)+len(spec))
	copy(all, spec)
	copy(all[len(spec):], p.portMappings)
	p.portMappings = all

	err = p.savePortMapping()
	if err != nil {
		p.Log(WARNING, "failed to persist new portmapping rules")
		// ignore the error
		err = nil
	}

	return nil
}

type portMappingCompare func(pm1, pm2 *apitypes.PortMapping) bool

func (p *XPod) removePortMapping(tbr []*apitypes.PortMapping, eq portMappingCompare) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if p.containerIP == "" || len(p.portMappings) == 0 || len(tbr) == 0 {
		return nil
	}

	rm := make([]*apitypes.PortMapping, 0, len(p.portMappings))
	other := make([]*apitypes.PortMapping, 0, len(p.portMappings))

	for _, pm := range p.portMappings {
		selected := false
		for _, sel := range tbr {
			if eq(pm, sel) {
				rm = append(rm, pm)
				selected = true
				break
			}
		}
		if !selected {
			other = append(other, pm)
		}
	}

	if len(rm) == 0 {
		p.Log(DEBUG, "no portmapping to be removed by %v", tbr)
		return nil
	}

	act, err := translatePortMapping(rm)
	if err != nil {
		p.Log(ERROR, "failed to generate removing rules: %v", err)
		return err
	}

	var extPrefix []string
	if p.globalSpec.PortmappingWhiteLists != nil &&
		len(p.globalSpec.PortmappingWhiteLists.InternalNetworks) > 0 &&
		len(p.globalSpec.PortmappingWhiteLists.ExternalNetworks) > 0 {
		extPrefix = p.globalSpec.PortmappingWhiteLists.ExternalNetworks
	}
	postExec, err := portmapping.ReleasePortMaps(p.containerIP, extPrefix, act)
	if err != nil {
		p.Log(ERROR, "failed to clean up rules: %v", err)
		return err
	}
	if len(postExec) > 0 {
		// don't need to release prestartExec here, it is not persistent
		if p.sandbox != nil {
			for _, ex := range postExec {
				_, stderr, err := p.sandbox.HyperstartExecSync(ex, nil)
				if err != nil {
					p.Log(ERROR, "failed to setup inSandbox mapping: %v [ %s", err, string(stderr))
					return err
				}
			}
		}
	}

	p.portMappings = other
	err = p.savePortMapping()
	if err != nil {
		p.Log(WARNING, "failed to persist removed portmapping rules")
		// ignore the error
		err = nil
	}

	return err
}

func (p *XPod) RemovePortMappingByDest(spec []*apitypes.PortMapping) error {
	return p.removePortMapping(spec, func(pm1, pm2 *apitypes.PortMapping) bool {
		return pm1.SameDestWith(pm2)
	})
}

func (p *XPod) RemovePortMappingStricted(spec []*apitypes.PortMapping) error {
	return p.removePortMapping(spec, func(pm1, pm2 *apitypes.PortMapping) bool {
		return pm1.EqualTo(pm2)
	})
}

func (p *XPod) ListPortMappings() []*apitypes.PortMapping {
	p.resourceLock.Lock()
	res := make([]*apitypes.PortMapping, len(p.portMappings))
	copy(res, p.portMappings)
	p.resourceLock.Unlock()
	return res
}
