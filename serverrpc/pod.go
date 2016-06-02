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
