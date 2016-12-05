package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ServiceList implements GET /service/list
func (s *ServerRPC) ServiceList(ctx context.Context, req *types.ServiceListRequest) (*types.ServiceListResponse, error) {
	glog.V(3).Infof("ServiceList with request %s", req.String())

	services, err := s.daemon.GetServices(req.PodID)
	if err != nil {
		glog.Errorf("ServiceList error: %v", err)
		return nil, err
	}

	return &types.ServiceListResponse{
		Services: services,
	}, nil
}

// ServiceAdd implements POST /service/list
func (s *ServerRPC) ServiceAdd(ctx context.Context, req *types.ServiceAddRequest) (*types.ServiceAddResponse, error) {
	glog.V(3).Infof("ServiceAdd with request %s", req.String())

	err := s.daemon.AddService(req.PodID, req.Services)
	if err != nil {
		glog.Errorf("ServiceAdd error: %v", err)
		return nil, err
	}
	return &types.ServiceAddResponse{}, nil
}

// ServiceDelete implements DELETE /service
func (s *ServerRPC) ServiceDelete(ctx context.Context, req *types.ServiceDelRequest) (*types.ServiceDelResponse, error) {
	glog.V(3).Infof("ServiceDelete with request %s", req.String())

	err := s.daemon.DeleteService(req.PodID, req.Services)
	if err != nil {
		glog.Errorf("ServiceDelete error: %v", err)
		return nil, err
	}
	return &types.ServiceDelResponse{}, nil
}

// ServiceUpdate implements UPDATE /service
func (s *ServerRPC) ServiceUpdate(ctx context.Context, req *types.ServiceUpdateRequest) (*types.ServiceUpdateResponse, error) {
	glog.V(3).Infof("ServiceUpdate with request %s", req.String())

	err := s.daemon.UpdateService(req.PodID, req.Services)
	if err != nil {
		glog.Errorf("ServiceUpdate error: %v", err)
		return nil, err
	}
	return &types.ServiceUpdateResponse{}, nil
}
