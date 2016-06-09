package container

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/docker/docker/builder/dockerfile"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/runconfig"
	"github.com/docker/docker/utils"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	timetypes "github.com/docker/engine-api/types/time"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/hyperd/server/httputils"
	"golang.org/x/net/context"
)

func (c *containerRouter) getContainerInfo(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	data, err := c.backend.CmdGetContainerInfo(r.Form.Get("container"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (c *containerRouter) getContainerLogs(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	// Args are validated before the stream starts because when it starts we're
	// sending HTTP 200 by writing an empty chunk of data to tell the client that
	// daemon is going to stream. By sending this initial HTTP 200 we can't report
	// any error after the stream starts (i.e. container not found, wrong parameters)
	// with the appropriate status code.
	stdout, stderr := httputils.BoolValue(r, "stdout"), httputils.BoolValue(r, "stderr")
	if !(stdout || stderr) {
		return fmt.Errorf("Bad parameters: you must choose at least one stream")
	}

	var since time.Time
	if r.Form.Get("since") != "" {
		s, n, err := timetypes.ParseTimestamps(r.Form.Get("since"), 0)
		if err != nil {
			return err
		}
		since = time.Unix(s, n)
	}

	var closeNotifier <-chan bool
	if notifier, ok := w.(http.CloseNotifier); ok {
		closeNotifier = notifier.CloseNotify()
	}

	containerName := r.Form.Get("container")
	/*
		if !s.backend.Exists(containerName) {
			return derr.ErrorCodeNoSuchContainer.WithArgs(containerName)
		}
	*/
	// write an empty chunk of data (this is to ensure that the
	// HTTP Response is sent immediately, even if the container has
	// not yet produced any data)
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	output := ioutils.NewWriteFlusher(w)
	defer output.Close()

	logsConfig := &daemon.ContainerLogsConfig{
		Follow:     httputils.BoolValue(r, "follow"),
		Timestamps: httputils.BoolValue(r, "timestamps"),
		Since:      since,
		Tail:       r.Form.Get("tail"),
		UseStdout:  stdout,
		UseStderr:  stderr,
		OutStream:  output,
		Stop:       closeNotifier,
	}

	if err := c.backend.CmdGetContainerLogs(containerName, logsConfig); err != nil {
		// The client may be expecting all of the data we're sending to
		// be multiplexed, so send it through OutStream, which will
		// have been set up to handle that if needed.
		fmt.Fprintf(logsConfig.OutStream, "Error running logs job: %s\n", utils.GetErrorMessage(err))
	}

	return nil
}

func (c *containerRouter) postContainerCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	if err := httputils.CheckForJSON(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	if podId == "" {
		return fmt.Errorf("podId is required to create a new container")
	}

	containerArgs, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	glog.V(1).Infof("Create container %s in pod %s", string(containerArgs), podId)

	containterID, err := c.backend.CmdCreateContainer(podId, containerArgs)
	if err != nil {
		return err
	}

	v := &engine.Env{}
	v.SetJson("ID", containterID)
	return v.WriteJSON(w, http.StatusCreated)
}

func (c *containerRouter) postContainerKill(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	sigterm := int64(15)
	cname := r.Form.Get("container")
	signal, err := httputils.Int64ValueOrDefault(r, "signal", sigterm)
	if err != nil {
		signal = sigterm
	}

	env, err := c.backend.CmdKillContainer(cname, signal)

	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusCreated)
}

func (c *containerRouter) postContainerStop(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	cname := r.Form.Get("container")
	env, err := c.backend.CmdStopContainer(cname)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusCreated)
}

func (c *containerRouter) postContainerCommit(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	cname := r.Form.Get("container")
	pause := httputils.BoolValue(r, "pause")

	config, _, _, err := runconfig.DecodeContainerConfig(r.Body)
	if err != nil && err != io.EOF { //Do not fail if body is empty.
		return err
	}
	if config == nil {
		config = &container.Config{}
	}

	newConfig, err := dockerfile.BuildFromConfig(config, r.Form["changes"])
	if err != nil {
		return err
	}

	commitCfg := &types.ContainerCommitConfig{
		Pause:        pause,
		Repo:         r.Form.Get("repo"),
		Tag:          r.Form.Get("tag"),
		Author:       r.Form.Get("author"),
		Comment:      r.Form.Get("comment"),
		Config:       newConfig,
		MergeConfigs: true,
	}

	env, err := c.backend.CmdCommitImage(cname, commitCfg)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (c *containerRouter) postContainerRename(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	newName := r.Form.Get("newName")
	oldName := r.Form.Get("oldName")
	env, err := c.backend.CmdContainerRename(oldName, newName)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}
