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
		tailLines int
	)

	p, id, ok := daemon.PodList.GetByContainerIdOrName(container)
	if !ok {
		err = fmt.Errorf("cannot find container %s", container)
		glog.Error(err)
		return err
	}

	l := p.ContainerLogger(id)
	if l == nil {
		err = fmt.Errorf("cannot get logger for container %s", container)
		glog.Error(err)
		return err
	}

	logReader, ok := l.(logger.LogReader)
	if !ok {
		err = fmt.Errorf("container %s: logger not support read", container)
		glog.Error(err)
		return err
	}

	follow := config.Follow && p.IsContainerAlive(id)
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
	if !p.ContainerHasTty(id) {
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
