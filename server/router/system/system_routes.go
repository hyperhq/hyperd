package system

import (
	"encoding/json"
	"net/http"

	"github.com/docker/engine-api/types"
	"github.com/hyperhq/hyperd/server/httputils"
	"golang.org/x/net/context"
)

func pingHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	_, err := w.Write([]byte{'O', 'K'})
	return err
}

func (s *systemRouter) getInfo(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	env, err := s.backend.CmdSystemInfo()
	if err != nil {
		return err
	}

	return env.WriteJSON(w, http.StatusOK)
}

func (s *systemRouter) getVersion(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	env := s.backend.CmdSystemVersion()
	return env.WriteJSON(w, http.StatusOK)
}

func (s *systemRouter) postAuth(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	var config *types.AuthConfig
	err := json.NewDecoder(r.Body).Decode(&config)
	r.Body.Close()
	if err != nil {
		return err
	}
	status, err := s.backend.CmdAuthenticateToRegistry(config)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, &types.AuthResponse{Status: status})
}
