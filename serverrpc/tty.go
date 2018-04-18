package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// TTYResize resizes the tty of the specified container
func (s *ServerRPC) TTYResize(c context.Context, req *types.TTYResizeRequest) (*types.TTYResizeResponse, error) {
	glog.V(3).Infof("TTYResize with request %s", req.String())

	err := s.daemon.TtyResize(req.ContainerID, req.ExecID, int(req.Height), int(req.Width))
	if err != nil {
		glog.Errorf("TTYResize failed %v with request %s", err, req.String())
		return nil, err
	}

	glog.V(3).Infof("TTYResize done with request %s", req.String())
	return &types.TTYResizeResponse{}, nil
}
