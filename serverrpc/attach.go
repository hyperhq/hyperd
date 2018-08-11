package serverrpc

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"io"
)

func (s *ServerRPC) Attach(stream types.PublicAPI_AttachServer) error {
	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stream.Recv error: %v", err)
	}
	glog.V(3).Infof("Attach with ServerStream %s request %s", stream, req.String())

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	go func() {
		defer ir.Close()
		for {
			cmd, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				glog.Errorf("Receive from stream error: %v", err)
				return
			}

			n, err := iw.Write(cmd.Data)
			if err != nil {
				glog.Errorf("Write pipe error: %v", err)
				return
			}
			if n != len(cmd.Data) {
				glog.Errorf("Write data length is not enough, write: %d, success: %d", len(cmd.Data), n)
				return
			}
		}
	}()

	go func() {
		defer or.Close()
		for {
			res := make([]byte, 512)
			n, err := or.Read(res)
			if n > 0 {
				if err := stream.Send(&types.AttachMessage{Data: res[:n]}); err != nil {
					glog.Errorf("Send to stream error: %v", err)
					return
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				glog.Errorf("Read from pipe error: %v", err)
				return
			}
		}
	}()

	err = s.daemon.Attach(ir, ow, req.ContainerID)
	if err != nil {
		return fmt.Errorf("s.daemon.Attach with request %s error: %v", req.String(), err)
	}

	return nil
}
