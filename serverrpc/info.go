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
		glog.Errorf("GetPodInfo error: %v", err)
		return nil, err
	}

	return &types.PodInfoResponse{
		PodInfo: &info,
	}, nil
}

// ContainerInfo gets ContainerInfo by ID or name of container
func (s *ServerRPC) ContainerInfo(c context.Context, req *types.ContainerInfoRequest) (*types.ContainerInfoResponse, error) {
	glog.V(3).Infof("ContainerInfo with request %v", req.String())

	info, err := s.daemon.GetContainerInfo(req.Container)
	if err != nil {
		glog.Errorf("GetContainerInfo error: %v", err)
		return nil, err
	}

	return &types.ContainerInfoResponse{
		ContainerInfo: &info,
	}, nil
}
