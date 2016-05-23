package serverrpc

import (
	"bytes"
	"io"
	"time"

	timetypes "github.com/docker/engine-api/types/time"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/types"
)

func (s *ServerRPC) ContainerLogs(req *types.ContainerLogsRequest, stream types.PublicAPI_ContainerLogsServer) error {
	glog.V(3).Infof("ContainerLogs with request %s", req.String())

	var since time.Time
	if req.Since != "" {
		s, n, err := timetypes.ParseTimestamps(req.Since, 0)
		if err != nil {
			return err
		}
		since = time.Unix(s, n)
	}

	buffer := bytes.NewBuffer([]byte{})

	stop := make(chan bool, 1)

	logsConfig := &daemon.ContainerLogsConfig{
		Follow:     req.Follow,
		Timestamps: req.Timestamps,
		Since:      since,
		Tail:       req.Tail,
		UseStdout:  req.Stdout,
		UseStderr:  req.Stderr,
		OutStream:  buffer,
		Stop:       stop,
	}

	if logsConfig.Follow == true {
		go s.daemon.GetContainerLogs(req.Container, logsConfig)
	} else {
		err := s.daemon.GetContainerLogs(req.Container, logsConfig)
		if err != nil {
			glog.Errorf("ContainerLogs error: %v", err)
			return err
		}
	}

	eof := false
	for {
		s, err := buffer.ReadBytes(byte('\n'))
		if err == io.EOF {
			if logsConfig.Follow == false {
				eof = true
			}
		} else if err != nil {
			glog.Errorf("Read log stream error: %v", err)
			return err
		}

		if err := stream.Send(&types.ContainerLogsResponse{Log: s}); err != nil {
			stop <- true
			return err
		}

		if eof == true {
			break
		}
	}

	return nil
}
