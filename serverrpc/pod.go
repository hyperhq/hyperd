package serverrpc

import (
	"fmt"
	"io"

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

// PodStart starts a pod by podID
func (s *ServerRPC) PodStart(stream types.PublicAPI_PodStartServer) error {
	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	glog.V(3).Infof("PodStart with request %s", req.String())

	if req.Tag == "" {
		_, _, err := s.daemon.StartPod(nil, nil, req.PodID, req.VmID, req.Tag)
		if err != nil {
			glog.Errorf("StartPod failed: %v", err)
			return err
		}

		// Send an empty message to client, so client could wait for start complete
		if err := stream.Send(&types.PodStartMessage{}); err != nil {
			return err
		}

		return nil
	}

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	go func() {
		for {
			cmd, err := stream.Recv()
			if err != nil {
				return
			}

			if _, err := iw.Write(cmd.Data); err != nil {
				return
			}
		}

	}()

	go func() {
		for {
			res := make([]byte, 512)
			n, err := or.Read(res)
			if err != nil {
				return
			}

			if err := stream.Send(&types.PodStartMessage{Data: res[:n]}); err != nil {
				return
			}
		}
	}()

	if _, _, err := s.daemon.StartPod(ir, ow, req.PodID, req.VmID, req.Tag); err != nil {
		glog.Errorf("StartPod failed: %v", err)
		return err
	}

	return nil
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

// PodStop stops a pod
func (s *ServerRPC) PodStop(ctx context.Context, req *types.PodStopRequest) (*types.PodStopResponse, error) {
	glog.V(3).Infof("PodStop with request %s", req.String())

	code, cause, err := s.daemon.StopPod(req.PodID)
	if err != nil {
		glog.Errorf("StopPod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodStopResponse{
		Cause: cause,
		Code:  int32(code),
	}, nil
}

// PodSignal sends a singal to all containers of specified pod
func (s *ServerRPC) PodSignal(ctx context.Context, req *types.PodSignalRequest) (*types.PodSignalResponse, error) {
	glog.V(3).Infof("PodSignal with request %s", req.String())

	err := s.daemon.KillPodContainers(req.PodID, "", req.Signal)
	if err != nil {
		glog.Errorf("KillPodContainers %s with signal %d failed: %v", req.PodID, req.Signal, err)
		return nil, err
	}

	return &types.PodSignalResponse{}, nil
}

// PodPause pauses a pod
func (s *ServerRPC) PodPause(ctx context.Context, req *types.PodPauseRequest) (*types.PodPauseResponse, error) {
	glog.V(3).Infof("PodPause with request %s", req.String())

	err := s.daemon.PausePod(req.PodID)
	if err != nil {
		glog.Errorf("PausePod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodPauseResponse{}, nil
}

// PodUnpause unpauses a pod
func (s *ServerRPC) PodUnpause(ctx context.Context, req *types.PodUnpauseRequest) (*types.PodUnpauseResponse, error) {
	glog.V(3).Infof("PodUnpause with request %s", req.String())

	err := s.daemon.UnpausePod(req.PodID)
	if err != nil {
		glog.Errorf("UnpausePod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodUnpauseResponse{}, nil
}

func (s *ServerRPC) PodLabels(c context.Context, req *types.PodLabelsRequest) (*types.PodLabelsResponse, error) {
	glog.V(3).Infof("Set pod labels with request %v", req.String())

	err := s.daemon.SetPodLabels(req.PodID, req.override, req.labels)
	if err != nil {
		glog.Errorf("PodLabels error: %v", err)
		return nil, err
	}

	return &types.PodLabelsResponse{}, nil
}
