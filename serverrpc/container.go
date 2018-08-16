package serverrpc

import (
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerCreate creates a container by UserContainer spec
func (s *ServerRPC) ContainerCreate(ctx context.Context, req *types.ContainerCreateRequest) (*types.ContainerCreateResponse, error) {
	containerID, err := s.daemon.CreateContainerInPod(req.PodID, req.ContainerSpec)
	if err != nil {
		return nil, err
	}

	return &types.ContainerCreateResponse{
		ContainerID: containerID,
	}, nil
}

func (s *ServerRPC) ContainerStart(ctx context.Context, req *types.ContainerStartRequest) (*types.ContainerStartResponse, error) {
	err := s.daemon.StartContainer(req.ContainerId)
	if err != nil {
		return nil, err
	}

	return &types.ContainerStartResponse{}, nil
}

// ContainerStop implements POST /container/stop
func (s *ServerRPC) ContainerStop(c context.Context, req *types.ContainerStopRequest) (*types.ContainerStopResponse, error) {
	err := s.daemon.StopContainer(req.ContainerID, int(req.Timeout))
	if err != nil {
		return nil, err
	}

	return &types.ContainerStopResponse{}, nil
}

// ContainerRename rename a container
func (s *ServerRPC) ContainerRename(c context.Context, req *types.ContainerRenameRequest) (*types.ContainerRenameResponse, error) {
	err := s.daemon.ContainerRename(req.OldContainerName, req.NewContainerName)
	if err != nil {
		return nil, err
	}

	return &types.ContainerRenameResponse{}, nil
}

func (s *ServerRPC) ContainerRemove(ctx context.Context, req *types.ContainerRemoveRequest) (*types.ContainerRemoveResponse, error) {
	err := s.daemon.RemoveContainer(req.ContainerId)
	if err != nil {
		return nil, err
	}

	return &types.ContainerRemoveResponse{}, nil
}

// ContainerSignal sends a singal to specified container of specified pod
func (s *ServerRPC) ContainerSignal(ctx context.Context, req *types.ContainerSignalRequest) (*types.ContainerSignalResponse, error) {
	err := s.daemon.KillPodContainers(req.PodID, req.ContainerID, req.Signal)
	if err != nil {
		return nil, err
	}

	return &types.ContainerSignalResponse{}, nil
}
