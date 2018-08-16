package serverrpc

import (
	"net"

	"github.com/golang/glog"
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"time"
)

// ServerRPC is the main server for gRPC
type ServerRPC struct {
	server *grpc.Server
	daemon *daemon.Daemon
}

type re interface {
	String() string
}

func unaryLoger(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	reqMsg := req.(re).String()
	glog.V(3).Infof("%s with request %s", info.FullMethod, reqMsg)

	start := time.Now()
	resp, err = handler(ctx, req)
	elapsed := time.Now().Sub(start)

	if err == nil {
		glog.V(3).Infof("%s elapsed %s done %v with request %s", info.FullMethod, elapsed, resp.(re).String(), reqMsg)
	} else {
		glog.Errorf("%s elapsed %s failed %v with request %s", info.FullMethod, elapsed, err, reqMsg)
	}

	return resp, err
}

func streamLoger(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	glog.V(3).Infof("%s with ServerStream %v", info.FullMethod, ss)

	start := time.Now()
	err := handler(srv, ss)
	elapsed := time.Now().Sub(start)

	if err == nil {
		glog.V(3).Infof("%s elapsed %s done with ServerStream %v", info.FullMethod, elapsed, ss)
	} else {
		glog.Errorf("%s elapsed %s failed %v with ServerStream %v", info.FullMethod, elapsed, err, ss)
	}

	return err
}

// NewServerRPC creates a new ServerRPC
func NewServerRPC(d *daemon.Daemon) *ServerRPC {
	s := &ServerRPC{
		server: grpc.NewServer(grpc.UnaryInterceptor(unaryLoger), grpc.StreamInterceptor(streamLoger)),
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
