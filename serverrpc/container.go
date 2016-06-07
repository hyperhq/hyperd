package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerCreate creates a container by UserContainer spec
func (s *ServerRPC) ContainerCreate(ctx context.Context, req *types.ContainerCreateRequest) (*types.ContainerCreateResponse, error) {
	glog.V(3).Infof("ContainerCreate with request %s", req.String())

	containerID, err := s.daemon.CreateContainerInPod(req.PodID, req.ContainerSpec)
	if err != nil {
		glog.Errorf("CreateContainerInPod failed: %v", err)
		return nil, err
	}

	return &types.ContainerCreateResponse{
		ContainerID: containerID,
	}, nil
}
