package serverrpc

import (
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
		return err
	}
	glog.V(3).Infof("Attach with request %s", req.String())

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	go func() {
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

	return s.daemon.Attach(ir, ow, req.ContainerID)
}
