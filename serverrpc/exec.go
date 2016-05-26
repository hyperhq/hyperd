package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// Wait gets exitcode by container and processId
func (s *ServerRPC) Wait(c context.Context, req *types.WaitRequest) (*types.WaitResponse, error) {
	glog.V(3).Infof("Wait with request %v", req.String())

	//FIXME need update if param NoHang is enabled
	code, err := s.daemon.ExitCode(req.Container, req.ProcessId)
	if err != nil {
		glog.Errorf("Wait error: %v", err)
		return nil, err
	}

	return &types.WaitResponse{
		ExitCode: int32(code),
	}, nil
}
