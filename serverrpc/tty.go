package serverrpc

import (
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// TTYResize resizes the tty of the specified container
func (s *ServerRPC) TTYResize(c context.Context, req *types.TTYResizeRequest) (*types.TTYResizeResponse, error) {
	err := s.daemon.TtyResize(req.ContainerID, req.ExecID, int(req.Height), int(req.Width))
	if err != nil {
		return nil, err
	}

	return &types.TTYResizeResponse{}, nil
}
