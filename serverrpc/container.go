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
		glog.Errorf("ContainerCreate failed %v with request %s: %v", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("ContainerCreate done with request %s", req.String())

	return &types.ContainerCreateResponse{
		ContainerID: containerID,
	}, nil
}

func (s *ServerRPC) ContainerStart(ctx context.Context, req *types.ContainerStartRequest) (*types.ContainerStartResponse, error) {
	glog.V(3).Info("ContainerStart with request %s", req.String())
	err := s.daemon.StartContainer(req.ContainerId)
	if err != nil {
		glog.Errorf("ContainerStart failed %v with request %s", err, req.String())
		return nil, err
	}
	glog.V(3).Info("ContainerStart done with request %s", req.String())
	return &types.ContainerStartResponse{}, nil
}

// ContainerStop implements POST /container/stop
func (s *ServerRPC) ContainerStop(c context.Context, req *types.ContainerStopRequest) (*types.ContainerStopResponse, error) {
	glog.V(3).Infof("ContainerStop with request %v", req.String())

	err := s.daemon.StopContainer(req.ContainerID, int(req.Timeout))
	if err != nil {
		glog.Errorf("ContainerStop failed %v with request %v", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("ContainerStop done with request %v", req.String())

	return &types.ContainerStopResponse{}, nil
}

// ContainerRename rename a container
func (s *ServerRPC) ContainerRename(c context.Context, req *types.ContainerRenameRequest) (*types.ContainerRenameResponse, error) {
	glog.V(3).Infof("ContainerRename with request %v", req.String())

	err := s.daemon.ContainerRename(req.OldContainerName, req.NewContainerName)
	if err != nil {
		glog.Errorf("ContainerRename failed %v with request %v", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("ContainerRename done with request %v", req.String())

	return &types.ContainerRenameResponse{}, nil
}

func (s *ServerRPC) ContainerRemove(ctx context.Context, req *types.ContainerRemoveRequest) (*types.ContainerRemoveResponse, error) {
	glog.V(3).Info("ContainerRemove with request %s", req.String())
	err := s.daemon.RemoveContainer(req.ContainerId)
	if err != nil {
		glog.Errorf("ContainerRemove failed %v with request %s", err, req.String())
		return nil, err
	}
	glog.V(3).Info("ContainerRemove done with request %s", req.String())
	return &types.ContainerRemoveResponse{}, nil
}

// ContainerSignal sends a singal to specified container of specified pod
func (s *ServerRPC) ContainerSignal(ctx context.Context, req *types.ContainerSignalRequest) (*types.ContainerSignalResponse, error) {
	glog.V(3).Infof("ContainerSignal with request %s", req.String())

	err := s.daemon.KillPodContainers(req.PodID, req.ContainerID, req.Signal)
	if err != nil {
		glog.Errorf("ContainerSignal failed %v done with request %s", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("ContainerSignal done with request %s", req.String())
	return &types.ContainerSignalResponse{}, nil
}
