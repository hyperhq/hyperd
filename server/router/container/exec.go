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

	code, err := s.backend.CmdExitCode(r.Form.Get("container"), r.Form.Get("tag"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, code)
}

func (s *containerRouter) postContainerExec(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	key := r.Form.Get("type")
	id := r.Form.Get("value")
	command := r.Form.Get("command")
	tag := r.Form.Get("tag")
	tty := r.Form.Get("tty")
	terminal := tty == "yes" || tty == "true" || tty == "on"

	// Setting up the streaming http interface.
	inStream, outStream, err := httputils.HijackConnection(w)
	if err != nil {
		return err
	}
	defer httputils.CloseStreams(inStream, outStream)
	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	return s.backend.CmdExec(inStream, outStream.(io.WriteCloser), key, id, command, tag, terminal)
}

func (s *containerRouter) postContainerAttach(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	key := r.Form.Get("type")
	id := r.Form.Get("value")
	tag := r.Form.Get("tag")
	//remove := r.Form.Get("remove")

	// Setting up the streaming http interface.
	inStream, outStream, err := httputils.HijackConnection(w)
	if err != nil {
		return err
	}
	defer httputils.CloseStreams(inStream, outStream)

	fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")

	return s.backend.CmdAttach(inStream, outStream.(io.WriteCloser), key, id, tag)
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

	tag := r.Form.Get("tag")
	podId := r.Form.Get("id")

	return s.backend.CmdTtyResize(podId, tag, height, width)
}
