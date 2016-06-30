package daemon

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/types"
)

// ContainerLogsConfig holds configs for logging operations. Exists
// for users of the daemon to to pass it a logging configuration.
type ContainerLogsConfig struct {
	// if true stream log output
	Follow bool
	// if true include timestamps for each line of log output
	Timestamps bool
	// return that many lines of log output from the end
	Tail string
	// filter logs by returning on those entries after this time
	Since time.Time
	// whether or not to show stdout and stderr as well as log entries.
	UseStdout, UseStderr bool
	OutStream            io.Writer
	Stop                 <-chan bool
}

func (daemon *Daemon) GetContainerLogs(container string, config *ContainerLogsConfig) (err error) {
	var (
		pod       *Pod
		cidx      int
		tailLines int
	)

	pod, cidx, err = daemon.GetPodByContainerIdOrName(container)
	if err != nil {
		return err
	}

	err = pod.getLogger(daemon)
	if err != nil {
		return err
	}

	logReader, ok := pod.PodStatus.Containers[cidx].Logs.Driver.(logger.LogReader)
	if !ok {
		return fmt.Errorf("logger not support read")
	}

	follow := config.Follow && (pod.PodStatus.Status == types.S_POD_RUNNING)
	tailLines, err = strconv.Atoi(config.Tail)
	if err != nil {
		tailLines = -1
	}

	readConfig := logger.ReadConfig{
		Since:  config.Since,
		Tail:   tailLines,
		Follow: follow,
	}

	logs := logReader.ReadLogs(readConfig)

	wf := ioutils.NewWriteFlusher(config.OutStream)
	defer wf.Close()
	wf.Flush()

	var outStream io.Writer = wf
	errStream := outStream
	if !pod.Spec.Containers[cidx].Tty {
		errStream = stdcopy.NewStdWriter(outStream, stdcopy.Stderr)
		outStream = stdcopy.NewStdWriter(outStream, stdcopy.Stdout)
	}

	for {
		select {
		case <-config.Stop:
			return nil
		case e := <-logs.Err:
			glog.Errorf("Error streaming logs: %v", e)
			return nil
		case msg, ok := <-logs.Msg:
			if !ok {
				glog.V(1).Info("logs: end stream")
				logs.Close()
				return nil
			}
			logLine := msg.Line
			if config.Timestamps {
				logLine = append([]byte(msg.Timestamp.Format(logger.TimeFormat)+" "), logLine...)
			}
			if msg.Source == "stdout" && config.UseStdout {
				glog.V(2).Info("print stdout log: ", logLine)
				_, err := outStream.Write(logLine)
				if err != nil {
					return nil
				}
			}
			if msg.Source == "stderr" && config.UseStderr {
				glog.V(2).Info("print stderr log: ", logLine)
				_, err := errStream.Write(logLine)
				if err != nil {
					return nil
				}
			}
		}
	}
}
