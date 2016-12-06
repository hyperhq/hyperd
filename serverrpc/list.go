package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerList implements GET /list?item=container
func (s *ServerRPC) ContainerList(ctx context.Context, req *types.ContainerListRequest) (*types.ContainerListResponse, error) {
	glog.V(3).Infof("ContainerList with request %s", req.String())

	containerList, err := s.daemon.ListContainers(req.PodID, req.VmID, req.Auxiliary)
	if err != nil {
		glog.Errorf("ContainerList error: %v", err)
		return nil, err
	}

	return &types.ContainerListResponse{
		ContainerList: containerList,
	}, nil
}

// PodList implements GET /list?item=pod
func (s *ServerRPC) PodList(ctx context.Context, req *types.PodListRequest) (*types.PodListResponse, error) {
	glog.V(3).Infof("PodList with request %s", req.String())

	podList, err := s.daemon.ListPods(req.PodID, req.VmID)
	if err != nil {
		glog.Errorf("PodList error: %v", err)
		return nil, err
	}

	return &types.PodListResponse{
		PodList: podList,
	}, nil
}

// VMList implements GET /list?item=vm
func (s *ServerRPC) VMList(ctx context.Context, req *types.VMListRequest) (*types.VMListResponse, error) {
	glog.V(3).Infof("VMList with request %s", req.String())

	vmList, err := s.daemon.ListVMs(req.PodID, req.VmID)
	if err != nil {
		glog.Errorf("VmList error: %v", err)
		return nil, err
	}

	return &types.VMListResponse{
		VmList: vmList,
	}, nil
}
