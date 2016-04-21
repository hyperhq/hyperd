package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// PodInfo gets PodInfo by podID
func (s *ServerRPC) PodInfo(c context.Context, req *types.PodInfoRequest) (*types.PodInfoResponse, error) {
	glog.V(3).Infof("PodInfo with request %v", req.String())

	info, err := s.daemon.GetPodInfo(req.PodID)
	if err != nil {
		glog.Infof("GetPodInfo error: %v", err)
		return nil, err
	}

	return &types.PodInfoResponse{
		PodInfo: &info,
	}, nil
}
