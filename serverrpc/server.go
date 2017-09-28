package serverrpc

import (
	"net"

	"github.com/hyperhq/hypercontainer-utils/hlog"
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

// LogPrefix() belongs to the interface `github.com/hyperhq/hypercontainer-utils/hlog.LogOwner`, which helps `hlog.HLog` get
// proper prefix from the owner object.
func (s *ServerRPC) LogPrefix() string {
	return "[gRPC] "
}

// Log() employ `github.com/hyperhq/hypercontainer-utils/hlog.HLog` to add pod information to the log
func (s *ServerRPC) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, s, 1, args...)
}

func (s *ServerRPC) registerServer() {
	types.RegisterPublicAPIServer(s.server, s)
}

// Serve serves gRPC request by goroutines
func (s *ServerRPC) Serve(addr string) error {
	s.Log(hlog.DEBUG, "start server at %s", addr)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		s.Log(hlog.ERROR, "Failed to listen %s: %v", addr, err)
		return err
	}

	return s.server.Serve(l)
}

// Stop stops gRPC server
func (s *ServerRPC) Stop() {
	s.server.Stop()
}
