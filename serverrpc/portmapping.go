package serverrpc

import (
	"fmt"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// PortMappingList get a list of PortMappings
func (s *ServerRPC) PortMappingList(ctx context.Context, req *types.PortMappingListRequest) (*types.PortMappingListResponse, error) {
	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		return nil, fmt.Errorf("Pod not found")
	}

	return &types.PortMappingListResponse{
		PortMappings: p.ListPortMappings(),
	}, nil
}

// PortMappingAdd add a list of PortMapping rules to a Pod
func (s *ServerRPC) PortMappingAdd(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		return nil, fmt.Errorf("Pod not found")
	}

	err := p.AddPortMapping(req.PortMappings)
	if err != nil {
		return nil, fmt.Errorf("p.AddPortMapping error: %v", err)
	}

	return &types.PortMappingModifyResponse{}, nil
}

// PortMappingDel remove a list of PortMapping rules from a Pod
func (s *ServerRPC) PortMappingDel(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		return nil, fmt.Errorf("Pod not found")
	}

	err := p.RemovePortMappingByDest(req.PortMappings)
	if err != nil {
		return nil, fmt.Errorf("p.RemovePortMappingByDest error: %v", err)
	}

	return &types.PortMappingModifyResponse{}, nil
}
