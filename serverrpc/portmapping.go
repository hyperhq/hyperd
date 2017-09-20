package serverrpc

import (
	"errors"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// PortMappingList get a list of PortMappings
func (s *ServerRPC) PortMappingList(ctx context.Context, req *types.PortMappingListRequest) (*types.PortMappingListResponse, error) {
	s.Log(hlog.TRACE, "PortMappingList with request %s", req.String())

	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		s.Log(hlog.INFO, "PortMappingList: pod %s not found", req.PodID)
		return nil, errors.New("Pod not found")
	}

	return &types.PortMappingListResponse{
		PortMappings: p.ListPortMappings(),
	}, nil
}

// PortMappingAdd add a list of PortMapping rules to a Pod
func (s *ServerRPC) PortMappingAdd(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	s.Log(hlog.TRACE, "PortMappingAdd with request %s", req.String())

	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		s.Log(hlog.INFO, "PortMappingAdd: pod %s not found", req.PodID)
		return nil, errors.New("Pod not found")
	}

	err := p.AddPortMapping(req.PortMappings)
	if err != nil {
		s.Log(hlog.ERROR, "failed to add port mappings: %v", err)
		return nil, err
	}
	return &types.PortMappingModifyResponse{}, nil
}

// PortMappingDel remove a list of PortMapping rules from a Pod
func (s *ServerRPC) PortMappingDel(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	s.Log(hlog.TRACE, "PortMappingDel with request %s", req.String())

	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		s.Log(hlog.INFO, "PortMappingDel: pod %s not found", req.PodID)
		return nil, errors.New("Pod not found")
	}

	err := p.RemovePortMappingByDest(req.PortMappings)
	if err != nil {
		s.Log(hlog.ERROR, "failed to add port mappings: %v", err)
		return nil, err
	}
	return &types.PortMappingModifyResponse{}, nil
}
