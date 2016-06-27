package container

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/server/httputils"
	"golang.org/x/net/context"
)

func (s *containerRouter) getExitCode(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	code, err := s.backend.CmdExitCode(r.Form.Get("container"), r.Form.Get("exec"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, code)
}

func (s *containerRouter) postContainerExecCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	id := r.Form.Get("container")
	command := r.Form.Get("command")
	tty := r.Form.Get("tty")
	terminal := tty == "yes" || tty == "true" || tty == "on"

	execId, err := s.backend.CreateExec(id, command, terminal)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusCreated, execId)
}

func (s *containerRouter) postContainerExecStart(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	id := r.Form.Get("container")
	execId := r.Form.Get("exec")

	// Setting up the streaming http interface.
	inStream, outStream, err := httputils.HijackConnection(w)
	if err != nil {
		return err
	}
	defer httputils.CloseStreams(inStream, outStream)
	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	return s.backend.StartExec(inStream, outStream.(io.WriteCloser), id, execId)
}

func (s *containerRouter) postContainerAttach(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	container := r.Form.Get("container")
	// Setting up the streaming http interface.
	inStream, outStream, err := httputils.HijackConnection(w)
	if err != nil {
		return err
	}
	defer httputils.CloseStreams(inStream, outStream)

	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	return s.backend.CmdAttach(inStream, outStream.(io.WriteCloser), container)
}

func (s *containerRouter) postTtyResize(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	height, err := strconv.Atoi(r.Form.Get("h"))
	if err != nil {
		return err
	}
	width, err := strconv.Atoi(r.Form.Get("w"))
	if err != nil {
		return err
	}

	containerId := r.Form.Get("container")
	execId := r.Form.Get("exec")

	return s.backend.CmdTtyResize(containerId, execId, height, width)
}
