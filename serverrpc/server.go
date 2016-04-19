package serverrpc

import (
	"net"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/types"
	"google.golang.org/grpc"
)

// ServerRPC is the main server for gRPC
type ServerRPC struct {
	server *grpc.Server
	daemon *daemon.Daemon
}

// NewServerRPC creates a new ServerRPC
func NewServerRPC(d *daemon.Daemon) *ServerRPC {
	s := &ServerRPC{
		server: grpc.NewServer(),
		daemon: d,
	}
	s.registerServer()
	return s
}

func (s *ServerRPC) registerServer() {
	types.RegisterPublicAPIServer(s.server, s)
}

// Serve serves gRPC request by goroutines
func (s *ServerRPC) Serve(addr string) error {
	glog.V(1).Infof("Start gRPC server at %s", addr)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		glog.Fatalf("Failed to listen %s: %v", addr, err)
		return err
	}

	return s.server.Serve(l)
}

// Stop stops gRPC server
func (s *ServerRPC) Stop() {
	s.server.Stop()
}
