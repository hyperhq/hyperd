package pod

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

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
	pod := r.Form.Get("pod")
	vm := r.Form.Get("vm")

	glog.V(1).Infof("List type is %s, specified pod: [%s], specified vm: [%s]", item, pod, vm)

	env, err := p.backend.CmdList(item, pod, vm)
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
	glog.V(1).Infof("Args string is %s", string(podArgs))

	env, err := p.backend.CmdCreatePod(string(podArgs))
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

	podId := r.Form.Get("podId")

	env, err := p.backend.CmdStartPod(podId)
	if err != nil {
		return err
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

// port mappings
func (p *podRouter) getPortMappings(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	env, err := p.backend.CmdListPortMappings(vars["id"])
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (p *podRouter) putPortMappings(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	pms, _ := ioutil.ReadAll(r.Body)
	switch vars["action"] {
	case "add":
		_, err := p.backend.CmdAddPortMappings(vars["id"], pms)
		if err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
	case "delete":
		_, err := p.backend.CmdDeletePortMappings(vars["id"], pms)
		if err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Only add or delete operation are permitted"))
		return nil
	}
	return nil
}
