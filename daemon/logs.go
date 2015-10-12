package daemon

import (
	"fmt"
	"strconv"
	"time"

	"github.com/docker/docker/daemon/logger"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

type logsCmdConfig struct {
	container  string
	follow     bool
	timestamps bool
	tail       string
	since      time.Time
	stdout     bool
	stderr     bool
}

func readLogsConfig(args []string) (*logsCmdConfig, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("container id or name must be provided")
	}

	cfg := &logsCmdConfig{
		container: args[0],
	}

	if len(args) > 1 {
		cfg.tail = args[1]
	}
	if len(args) > 2 {
		s, err := strconv.ParseInt(args[2], 10, 64)
		if err == nil {
			cfg.since = time.Unix(s, 0)
		}
	}

	for _, s := range args[3:] {
		switch s {
		case "follow":
			cfg.follow = true
		case "timestamp":
			cfg.timestamps = true
		case "stdout":
			cfg.stdout = true
		case "stderr":
			cfg.stderr = true
		}
	}

	return cfg, nil
}

func (daemon *Daemon) CmdLogs(job *engine.Job) (err error) {
	var (
		config    *logsCmdConfig
		pod       *Pod
		cidx      int
		tailLines int
	)
	config, err = readLogsConfig(job.Args)
	if err != nil {
		glog.Warningf("log args parsing error: %v", err)
		return
	}

	if !(config.stdout || config.stderr) {
		return fmt.Errorf("You must choose at least one stream")
	}

	outStream := job.Stdout
	errStream := outStream

	pod, cidx, err = daemon.GetPodByContainerIdOrName(config.container)
	if err != nil {
		return err
	}

	err = pod.getLogger(daemon)
	if err != nil {
		return err
	}

	logReader, ok := pod.status.Containers[cidx].Logs.Driver.(logger.LogReader)
	if !ok {
		return fmt.Errorf("logger not suppert read")
	}

	follow := config.follow && (pod.status.Status == types.S_POD_RUNNING)
	tailLines, err = strconv.Atoi(config.tail)
	if err != nil {
		tailLines = -1
	}

	readConfig := logger.ReadConfig{
		Since:  config.since,
		Tail:   tailLines,
		Follow: follow,
	}

	logs := logReader.ReadLogs(readConfig)
	for {
		select {
		case e := <-logs.Err:
			glog.Errorf("Error streaming logs: %v", e)
			return nil
		case msg, ok := <-logs.Msg:
			if !ok {
				glog.V(1).Info("logs: end stream")
				return nil
			}
			logLine := msg.Line
			if config.timestamps {
				logLine = append([]byte(msg.Timestamp.Format(logger.TimeFormat)+" "), logLine...)
			}
			if msg.Source == "stdout" && config.stdout {
				glog.V(2).Info("print stdout log: ", logLine)
				_, err := outStream.Write(logLine)
				if err != nil {
					return nil
				}
			}
			if msg.Source == "stderr" && config.stderr {
				glog.V(2).Info("print stderr log: ", logLine)
				_, err := errStream.Write(logLine)
				if err != nil {
					return nil
				}
			}
		}
	}
}
