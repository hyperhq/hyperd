package serverrpc

import (
	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

func (s *ServerRPC) Auth(ctx context.Context, req *types.AuthRequest) (*types.AuthResponse, error) {
	authconfig := dockertypes.AuthConfig{
		Username:      req.Username,
		Password:      req.Password,
		Auth:          req.Auth,
		Email:         req.Email,
		ServerAddress: req.Serveraddress,
		RegistryToken: req.Registrytoken,
	}
	status, err := s.daemon.CmdAuthenticateToRegistry(&authconfig)
	if err != nil {
		glog.Errorf("auth error %v", err)
	}
	return &types.AuthResponse{
		Status: status,
	}, err
}
