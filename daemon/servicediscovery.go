package daemon

import (
	"fmt"

	apitypes "github.com/hyperhq/hyperd/types"
)

func (daemon *Daemon) AddService(podId string, srvs []*apitypes.UserService) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	return p.AddService(srvs)
}

func (daemon *Daemon) UpdateService(podId string, srvs []*apitypes.UserService) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	return p.UpdateService(srvs)
}

func (daemon *Daemon) DeleteService(podId string, srvs []*apitypes.UserService) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	return p.DeleteService(srvs)
}

func (daemon *Daemon) GetServices(podId string) ([]*apitypes.UserService, error) {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return nil, fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	return p.GetServices()
}
