package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"golang.org/x/net/context"
)

const (
	GRPC_API_VERSION = "0.1.0"
)

// PodInfo gets PodInfo by podID
func (s *ServerRPC) PodInfo(c context.Context, req *types.PodInfoRequest) (*types.PodInfoResponse, error) {
	glog.V(3).Infof("PodInfo with request %s", req.String())

	info, err := s.daemon.GetPodInfo(req.PodID)
	if err != nil {
		glog.Errorf("PodInfo failed %v with request %s", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("PodInfo done with request %s", req.String())
	return &types.PodInfoResponse{
		PodInfo: info,
	}, nil
}

// ContainerInfo gets ContainerInfo by ID or name of container
func (s *ServerRPC) ContainerInfo(c context.Context, req *types.ContainerInfoRequest) (*types.ContainerInfoResponse, error) {
	glog.V(3).Infof("ContainerInfo with request %s", req.String())

	info, err := s.daemon.GetContainerInfo(req.Container)
	if err != nil {
		glog.Errorf("ContainerInfo failed %v with request %s", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("ContainerInfo done with request %s", req.String())
	return &types.ContainerInfoResponse{
		ContainerInfo: info,
	}, nil
}

// Info gets CmdSystemInfo
func (s *ServerRPC) Info(c context.Context, req *types.InfoRequest) (*types.InfoResponse, error) {
	glog.V(3).Infof("Info with request %s", req.String())

	info, err := s.daemon.CmdSystemInfo()
	if err != nil {
		glog.Errorf("Info failed %v with request %s", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("Info done with request %s", req.String())
	return info, nil
}

// Version gets the version and apiVersion of hyperd
func (s *ServerRPC) Version(c context.Context, req *types.VersionRequest) (*types.VersionResponse, error) {
	return &types.VersionResponse{
		Version:    utils.VERSION,
		ApiVersion: GRPC_API_VERSION,
	}, nil
}

// Ping checks if hyperd is running (returns 'OK' on success)
func (s *ServerRPC) Ping(c context.Context, req *types.PingRequest) (*types.PingResponse, error) {
	return &types.PingResponse{HyperdStats: "OK"}, nil
}
