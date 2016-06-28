package pod

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/server/httputils"
	"golang.org/x/net/context"
)

func (p *podRouter) getPodInfo(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	data, err := p.backend.CmdGetPodInfo(r.Form.Get("podName"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (p *podRouter) getPodStats(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	data, err := p.backend.CmdGetPodStats(r.Form.Get("podId"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (p *podRouter) getList(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	item := r.Form.Get("item")
	auxiliary := httputils.BoolValue(r, "auxiliary")
	pod := r.Form.Get("pod")
	vm := r.Form.Get("vm")

	glog.V(1).Infof("List type is %s, specified pod: [%s], specified vm: [%s], list auxiliary pod: %v", item, pod, vm, auxiliary)

	env, err := p.backend.CmdList(item, pod, vm, auxiliary)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusCreated)
}

func (p *podRouter) postPodCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	if err := httputils.CheckForJSON(r); err != nil {
		return err
	}

	podArgs, _ := ioutil.ReadAll(r.Body)
	autoRemove := false
	if r.Form.Get("remove") == "yes" || r.Form.Get("remove") == "true" {
		autoRemove = true
	}
	glog.V(1).Infof("Args string is %s, autoremove %v", string(podArgs), autoRemove)

	env, err := p.backend.CmdCreatePod(string(podArgs), autoRemove)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusCreated)
}

func (p *podRouter) postPodLabels(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	labels := make(map[string]string)

	if err := json.Unmarshal([]byte(r.Form.Get("labels")), &labels); err != nil {
		return err
	}

	override := false
	if r.Form.Get("override") == "true" || r.Form.Get("override") == "yes" {
		override = true
	}

	env, err := p.backend.CmdSetPodLabels(podId, override, labels)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusCreated)
}

func (p *podRouter) postPodStart(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	attach := false
	podId := r.Form.Get("podId")
	vmId := r.Form.Get("vmId")
	if val := r.Form.Get("attach"); val == "yes" || val == "true" || val == "on" {
		attach = true
	}

	var (
		inStream  io.ReadCloser  = nil
		outStream io.WriteCloser = nil
	)

	if attach {
		// Setting up the streaming http interface.
		in, out, err := httputils.HijackConnection(w)
		if err != nil {
			return err
		}

		inStream = in
		outStream = out.(io.WriteCloser)
		defer httputils.CloseStreams(inStream, outStream)

		fmt.Fprintf(outStream, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
	}

	env, err := p.backend.CmdStartPod(inStream, outStream, podId, vmId, attach)
	if err != nil {
		return err
	}

	if attach {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) postPodStop(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	stopVm := r.Form.Get("stopVm")

	env, err := p.backend.CmdStopPod(podId, stopVm)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) postPodKill(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	var (
		podId     = r.Form.Get("podName")
		container = r.Form.Get("container")
	)

	sigterm := int64(15)
	signal, err := httputils.Int64ValueOrDefault(r, "signal", sigterm)
	if err != nil {
		signal = sigterm
	}

	env, err := p.backend.CmdKillPod(podId, container, signal)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) postPodPause(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")

	if err := p.backend.CmdPausePod(podId); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (p *podRouter) postPodUnpause(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	if err := p.backend.CmdUnpausePod(podId); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (p *podRouter) postVmCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	var (
		cpu   = 1
		mem   = 128
		async = false
		err   error
	)
	if value := r.Form.Get("cpu"); value != "" {
		cpu, err = strconv.Atoi(value)
		if err != nil {
			return err
		}
	}
	if value := r.Form.Get("mem"); value != "" {
		mem, err = strconv.Atoi(value)
		if err != nil {
			return err
		}
	}
	if r.Form.Get("async") == "yes" || r.Form.Get("async") == "true" {
		async = true
	}

	env, err := p.backend.CmdCreateVm(cpu, mem, async)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) deletePod(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	env, err := p.backend.CmdCleanPod(podId)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) deleteVm(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	vmId := r.Form.Get("vm")
	env, err := p.backend.CmdKillVm(vmId)
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}
