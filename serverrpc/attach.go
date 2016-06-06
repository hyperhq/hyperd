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
			if err != nil {
				return
			}
			if _, err := iw.Write(cmd.Data); err != nil {
				return
			}
		}
	}()

	go func() {
		for {
			res := make([]byte, 512)
			n, err := or.Read(res)
			if err != nil {
				return
			}
			if err := stream.Send(&types.AttachMessage{Data: res[:n]}); err != nil {
				return
			}
		}
	}()

	return s.daemon.Attach(ir, ow, "", req.ContainerID, req.Tag)
}
