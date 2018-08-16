package serverrpc

import (
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerList implements GET /list?item=container
func (s *ServerRPC) ContainerList(ctx context.Context, req *types.ContainerListRequest) (*types.ContainerListResponse, error) {
	containerList, err := s.daemon.ListContainers(req.PodID, req.VmID)
	if err != nil {
		return nil, err
	}

	return &types.ContainerListResponse{
		ContainerList: containerList,
	}, nil
}

// PodList implements GET /list?item=pod
func (s *ServerRPC) PodList(ctx context.Context, req *types.PodListRequest) (*types.PodListResponse, error) {
	podList, err := s.daemon.ListPods(req.PodID, req.VmID)
	if err != nil {
		return nil, err
	}

	return &types.PodListResponse{
		PodList: podList,
	}, nil
}

// VMList implements GET /list?item=vm
func (s *ServerRPC) VMList(ctx context.Context, req *types.VMListRequest) (*types.VMListResponse, error) {
	vmList, err := s.daemon.ListVMs(req.PodID, req.VmID)
	if err != nil {
		return nil, err
	}

	return &types.VMListResponse{
		VmList: vmList,
	}, nil
}
