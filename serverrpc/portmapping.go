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
		err := errors.New("Pod not found")
		s.Log(hlog.ERROR, "PortMappingList failed %v with request %s", err, req.String())
		return nil, err
	}

	s.Log(hlog.TRACE, "PortMappingList done with request %s", req.String())
	return &types.PortMappingListResponse{
		PortMappings: p.ListPortMappings(),
	}, nil
}

// PortMappingAdd add a list of PortMapping rules to a Pod
func (s *ServerRPC) PortMappingAdd(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	s.Log(hlog.TRACE, "PortMappingAdd with request %s", req.String())

	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		err := errors.New("Pod not found")
		s.Log(hlog.ERROR, "PortMappingAdd failed %v with request %s", err, req.String())
		return nil, err
	}

	err := p.AddPortMapping(req.PortMappings)
	if err != nil {
		s.Log(hlog.ERROR, "PortMappingAdd failed %v with request %s", err, req.String())
		return nil, err
	}
	s.Log(hlog.TRACE, "PortMappingAdd done with request %s", req.String())
	return &types.PortMappingModifyResponse{}, nil
}

// PortMappingDel remove a list of PortMapping rules from a Pod
func (s *ServerRPC) PortMappingDel(ctx context.Context, req *types.PortMappingModifyRequest) (*types.PortMappingModifyResponse, error) {
	s.Log(hlog.TRACE, "PortMappingDel with request %s", req.String())

	p, ok := s.daemon.PodList.Get(req.PodID)
	if !ok {
		err := errors.New("Pod not found")
		s.Log(hlog.ERROR, "PortMappingDel failed %v with request %s", err, req.String())
		return nil, err
	}

	err := p.RemovePortMappingByDest(req.PortMappings)
	if err != nil {
		s.Log(hlog.ERROR, "PortMappingDel failed %v with request %s", err, req.String())
		return nil, err
	}

	s.Log(hlog.TRACE, "PortMappingDel done with request %s", req.String())
	return &types.PortMappingModifyResponse{}, nil
}
