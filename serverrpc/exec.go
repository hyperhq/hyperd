package serverrpc

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
	"io"
)

func (s *ServerRPC) ExecCreate(ctx context.Context, req *types.ExecCreateRequest) (*types.ExecCreateResponse, error) {
	glog.V(3).Infof("create exec %v", req.String())

	cmd, err := json.Marshal(req.Command)
	if err != nil {
		return nil, err
	}

	execId, err := s.daemon.CreateExec(req.ContainerID, string(cmd), req.Tty)
	if err != nil {
		glog.Errorf("ExecCreate error: %v", err)
		return nil, err
	}

	return &types.ExecCreateResponse{
		ExecID: execId,
	}, nil
}

func (s *ServerRPC) ExecStart(stream types.PublicAPI_ExecStartServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	go func() {
		defer outReader.Close()
		buf := make([]byte, 32)
		for {
			nr, err := outReader.Read(buf)
			if nr > 0 {
				if err := stream.Send(&types.ExecStartResponse{buf[:nr]}); err != nil {
					glog.Errorf("Send to stream error: %v", err)
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				glog.Errorf("Read from pipe error: %v", err)
				return
			}
		}
	}()
	go func() {
		defer inWriter.Close()
		for {
			req, err := stream.Recv()
			if err != nil && err != io.EOF {
				glog.Errorf("Receive from stream error: %v", err)
				return
			}
			if req != nil && req.Stdin != nil {
				nw, ew := inWriter.Write(req.Stdin)
				if ew != nil {
					glog.Errorf("Write pipe error: %v", ew)
					return
				}
				if nw != len(req.Stdin) {
					glog.Errorf("Write data length is not enougt, write: %d success: %d", len(req.Stdin), nw)
					return
				}
			}
			if err == io.EOF {
				break
			}
		}
	}()

	err = s.daemon.StartExec(inReader, outWriter, req.ContainerID, req.ExecID)
	if err != nil {
		return err
	}
	return nil
}

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
