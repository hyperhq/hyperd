package libhyperstart

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"syscall"
	"time"

	hyperstartgrpc "github.com/hyperhq/runv/hyperstart/api/grpc"
	hyperstartjson "github.com/hyperhq/runv/hyperstart/api/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
)

type grpcBasedHyperstart struct {
	ctx  context.Context
	conn *grpc.ClientConn
	grpc hyperstartgrpc.HyperstartServiceClient
}

// NewGrpcBasedHyperstart create hyperstart interface with grpc protocol
func NewGrpcBasedHyperstart(hyperstartGRPCSock string) (Hyperstart, error) {
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(5 * time.Second)}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		},
		))
	conn, err := grpc.Dial(hyperstartGRPCSock, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &grpcBasedHyperstart{
		ctx:  context.Background(),
		conn: conn,
		grpc: hyperstartgrpc.NewHyperstartServiceClient(conn),
	}, nil
}

func (h *grpcBasedHyperstart) Close() {
	h.conn.Close()
}

func (h *grpcBasedHyperstart) LastStreamSeq() uint64 {
	return 0
}

func (h *grpcBasedHyperstart) APIVersion() (uint32, error) {
	return 4244, nil
}

func (h *grpcBasedHyperstart) PauseSync() error {
	// TODO:
	return nil
}

func (h *grpcBasedHyperstart) Unpause() error {
	// TODO:
	return nil
}

func (h *grpcBasedHyperstart) WriteFile(container, path string, data []byte) error {
	return fmt.Errorf("WriteFile() is unsupported on grpc based hyperstart API")
}

func (h *grpcBasedHyperstart) ReadFile(container, path string) ([]byte, error) {
	return nil, fmt.Errorf("ReadFile() is unsupported on grpc based hyperstart API")
}

func (h *grpcBasedHyperstart) AddRoute(routes []hyperstartjson.Route) error {
	req := &hyperstartgrpc.AddRouteRequest{}
	for _, r := range routes {
		req.Routes = append(req.Routes, &hyperstartgrpc.Route{
			Dest:    r.Dest,
			Gateway: r.Gateway,
			Device:  r.Device,
		})
	}
	_, err := h.grpc.AddRoute(h.ctx, req)
	return err
}

func (h *grpcBasedHyperstart) UpdateInterface(t InfUpdateType, dev, newName string, ipAddresses []hyperstartjson.IpAddress, mtu uint64) error {
	req := &hyperstartgrpc.UpdateInterfaceRequest{
		Type:    uint64(t),
		Device:  dev,
		NewName: newName,
		Mtu:     mtu,
	}
	for _, addr := range ipAddresses {
		req.IpAddresses = append(req.IpAddresses, &hyperstartgrpc.IpAddress{addr.IpAddress, addr.NetMask})
	}
	_, err := h.grpc.UpdateInterface(h.ctx, req)
	return err
}

func (h *grpcBasedHyperstart) WriteStdin(container, process string, data []byte) (int, error) {
	ret, err := h.grpc.WriteStdin(h.ctx, &hyperstartgrpc.WriteStreamRequest{
		Container: container,
		Process:   process,
		Data:      data,
	})
	if err == nil {
		return int(ret.Len), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (h *grpcBasedHyperstart) ReadStdout(container, process string, data []byte) (int, error) {
	ret, err := h.grpc.ReadStdout(h.ctx, &hyperstartgrpc.ReadStreamRequest{
		Container: container,
		Process:   process,
		Len:       uint32(len(data)),
	})
	if err == nil {
		copy(data, ret.Data)
		return len(ret.Data), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (h *grpcBasedHyperstart) ReadStderr(container, process string, data []byte) (int, error) {
	ret, err := h.grpc.ReadStderr(h.ctx, &hyperstartgrpc.ReadStreamRequest{
		Container: container,
		Process:   process,
		Len:       uint32(len(data)),
	})
	if err == nil {
		copy(data, ret.Data)
		return len(ret.Data), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (h *grpcBasedHyperstart) CloseStdin(container, process string) error {
	_, err := h.grpc.CloseStdin(h.ctx, &hyperstartgrpc.CloseStdinRequest{
		Container: container,
		Process:   process,
	})
	return err
}

func (h *grpcBasedHyperstart) TtyWinResize(container, process string, row, col uint16) error {
	_, err := h.grpc.TtyWinResize(h.ctx, &hyperstartgrpc.TtyWinResizeRequest{
		Container: container,
		Process:   process,
		Row:       uint32(row),
		Column:    uint32(col),
	})
	return err
}

func (h *grpcBasedHyperstart) OnlineCpuMem() error {
	_, err := h.grpc.OnlineCPUMem(h.ctx, &hyperstartgrpc.OnlineCPUMemRequest{})
	return err
}

func process4json2grpc(p *hyperstartjson.Process) *hyperstartgrpc.Process {
	envs := map[string]string{}
	for _, env := range p.Envs {
		envs[env.Env] = env.Value
	}
	return &hyperstartgrpc.Process{
		Id:       p.Id,
		User:     &hyperstartgrpc.User{Uid: p.User, Gid: p.Group, AdditionalGids: p.AdditionalGroups},
		Terminal: p.Terminal,
		Envs:     envs,
		Args:     p.Args,
		Workdir:  p.Workdir,
	}
}

func container4json2grpc(c *hyperstartjson.Container) *hyperstartgrpc.Container {
	if c.Fstype != "" || c.Addr != "" || len(c.Volumes) != 0 || len(c.Fsmap) != 0 {
		panic("unsupported rootfs(temporary)")
	}
	mounts := []*hyperstartgrpc.Mount{{Dest: "/", Source: "vm:/dev/hostfs/" + c.Image + "/" + c.Rootfs}}
	return &hyperstartgrpc.Container{
		Id:     c.Id,
		Mounts: mounts,
		Sysctl: c.Sysctl,
	}
}

func (h *grpcBasedHyperstart) NewContainer(c *hyperstartjson.Container) error {
	_, err := h.grpc.AddContainer(h.ctx, &hyperstartgrpc.AddContainerRequest{
		Container: container4json2grpc(c),
		Init:      process4json2grpc(c.Process),
	})
	return err
}

func (h *grpcBasedHyperstart) RestoreContainer(c *hyperstartjson.Container) error {
	// nothing to do
	return nil
}

func (h *grpcBasedHyperstart) AddProcess(container string, p *hyperstartjson.Process) error {
	_, err := h.grpc.AddProcess(h.ctx, &hyperstartgrpc.AddProcessRequest{
		Container: container,
		Process:   process4json2grpc(p),
	})
	return err
}

func (h *grpcBasedHyperstart) SignalProcess(container, process string, signal syscall.Signal) error {
	_, err := h.grpc.SignalProcess(h.ctx, &hyperstartgrpc.SignalProcessRequest{
		Container: container,
		Process:   process,
		Signal:    uint32(signal),
	})
	return err
}

// wait the process until exit. like waitpid()
// the state is saved until someone calls WaitProcess() if the process exited earlier
// the non-first call of WaitProcess() after process started MAY fail to find the process if the process exited earlier
func (h *grpcBasedHyperstart) WaitProcess(container, process string) int {
	ret, err := h.grpc.WaitProcess(h.ctx, &hyperstartgrpc.WaitProcessRequest{
		Container: container,
		Process:   process,
	})
	if err != nil {
		return -1
	}
	return int(ret.Status)
}

func (h *grpcBasedHyperstart) StartSandbox(pod *hyperstartjson.Pod) error {
	_, err := h.grpc.StartSandbox(h.ctx, &hyperstartgrpc.StartSandboxRequest{
		Hostname: pod.Hostname,
		Dns:      pod.Dns,
	})
	return err
}

func (h *grpcBasedHyperstart) DestroySandbox() error {
	_, err := h.grpc.DestroySandbox(h.ctx, &hyperstartgrpc.DestroySandboxRequest{})
	return err
}
