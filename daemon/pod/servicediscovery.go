package pod

import apitypes "github.com/hyperhq/hyperd/types"

func (p *XPod) GetServices() ([]*apitypes.UserService, error) {
	return p.services, nil
}

func (p *XPod) UpdateService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	return nil
}

func (p *XPod) AddService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	return nil
}

func (p *XPod) DeleteService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	return nil
}
