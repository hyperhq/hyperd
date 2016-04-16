package service

import (
	"net/http"

	"github.com/hyperhq/hyperd/server/httputils"
	"golang.org/x/net/context"
)

func (s *serviceRouter) getServices(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	data, err := s.backend.CmdGetServices(r.Form.Get("podId"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (s *serviceRouter) postServiceAdd(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	services := r.Form.Get("services")

	data, err := s.backend.CmdAddService(podId, services)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (s *serviceRouter) postServiceUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	services := r.Form.Get("services")

	data, err := s.backend.CmdUpdateService(podId, services)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}

func (s *serviceRouter) deleteService(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	podId := r.Form.Get("podId")
	services := r.Form.Get("services")

	data, err := s.backend.CmdDeleteService(podId, services)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, data)
}
