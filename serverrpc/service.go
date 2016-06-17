package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/runv/hypervisor/pod"
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

	result := make([]*types.UserService, 0, len(services))

	for _, service := range services {
		var hosts []*types.UserServiceBackend
		// deep copy service.Hosts to rpc.Hosts
		for _, host := range service.Hosts {
			rpcHost := &types.UserServiceBackend{HostIP: host.HostIP, HostPort: int32(host.HostPort)}
			hosts = append(hosts, rpcHost)
		}
		result = append(result, &types.UserService{
			ServiceIP:   service.ServiceIP,
			Protocol:    service.Protocol,
			ServicePort: int32(service.ServicePort),
			Hosts:       hosts,
		})
	}

	return &types.ServiceListResponse{
		Services: result,
	}, nil
}

// NOTE: I did not use a general deep copy to handle this convert because reflection is slow
func convertServiceRpcToRunvType(services []*types.UserService) []pod.UserService {
	result := make([]pod.UserService, 0, len(services))

	for _, service := range services {
		var hosts []pod.UserServiceBackend
		// deep copy rpc Hosts to runv Hosts
		for _, host := range service.Hosts {
			runvHost := pod.UserServiceBackend{HostIP: host.HostIP, HostPort: int(host.HostPort)}
			hosts = append(hosts, runvHost)
		}
		result = append(result, pod.UserService{
			ServiceIP:   service.ServiceIP,
			Protocol:    service.Protocol,
			ServicePort: int(service.ServicePort),
			Hosts:       hosts,
		})
	}

	return result
}

// ServiceAdd implements POST /service/list
func (s *ServerRPC) ServiceAdd(ctx context.Context, req *types.ServiceAddRequest) (*types.ServiceAddResponse, error) {
	glog.V(3).Infof("ServiceAdd with request %s", req.String())

	err := s.daemon.AddService(req.PodID, convertServiceRpcToRunvType(req.Services))
	if err != nil {
		glog.Errorf("ServiceAdd error: %v", err)
		return nil, err
	}
	return &types.ServiceAddResponse{}, nil
}

// ServiceDelete implements DELETE /service
func (s *ServerRPC) ServiceDelete(ctx context.Context, req *types.ServiceDelRequest) (*types.ServiceDelResponse, error) {
	glog.V(3).Infof("ServiceDelete with request %s", req.String())

	err := s.daemon.DeleteService(req.PodID, convertServiceRpcToRunvType(req.Services))
	if err != nil {
		glog.Errorf("ServiceDelete error: %v", err)
		return nil, err
	}
	return &types.ServiceDelResponse{}, nil
}

// ServiceUpdate implements UPDATE /service
func (s *ServerRPC) ServiceUpdate(ctx context.Context, req *types.ServiceUpdateRequest) (*types.ServiceUpdateResponse, error) {
	glog.V(3).Infof("ServiceUpdate with request %s", req.String())

	err := s.daemon.UpdateService(req.PodID, convertServiceRpcToRunvType(req.Services))
	if err != nil {
		glog.Errorf("ServiceUpdate error: %v", err)
		return nil, err
	}
	return &types.ServiceUpdateResponse{}, nil
}
