package serverrpc

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// PodCreate creates a pod by PodSpec
func (s *ServerRPC) PodCreate(ctx context.Context, req *types.PodCreateRequest) (*types.PodCreateResponse, error) {
	glog.V(3).Infof("PodCreate with request %s", req.String())

	pod, err := s.daemon.CreatePod(req.PodID, req.PodSpec)
	if err != nil {
		glog.Errorf("CreatePod failed: %v", err)
		return nil, err
	}

	return &types.PodCreateResponse{
		PodID: pod.Id,
	}, nil
}

// PodRemove removes a pod by podID
func (s *ServerRPC) PodRemove(ctx context.Context, req *types.PodRemoveRequest) (*types.PodRemoveResponse, error) {
	glog.V(3).Infof("PodRemove with request %s", req.String())

	if req.PodID == "" {
		return nil, fmt.Errorf("PodID is required for PodRemove")
	}

	code, cause, err := s.daemon.CleanPod(req.PodID)
	if err != nil {
		glog.Errorf("CleanPod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodRemoveResponse{
		Cause: cause,
		Code:  int32(code),
	}, nil
}
