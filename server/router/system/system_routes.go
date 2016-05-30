package system

import (
	"encoding/json"
	"net/http"

	"github.com/docker/engine-api/types"
	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/hyperd/server/httputils"
	"golang.org/x/net/context"
)

func pingHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	_, err := w.Write([]byte{'O', 'K'})
	return err
}

func (s *systemRouter) getInfo(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	info, err := s.backend.CmdSystemInfo()
	if err != nil {
		return err
	}

	env := &engine.Env{}
	status := [][2]string{}

	env.Set("ID", info.ID)
	env.SetInt("Containers", int(info.Containers))
	env.SetInt("Images", int(info.Images))
	env.Set("Driver", info.Driver)
	env.Set("DockerRootDir", info.DockerRootDir)
	env.Set("IndexServerAddress", info.IndexServerAddress)
	env.Set("ExecutionDriver", info.ExecutionDriver)
	env.SetInt64("MemTotal", info.MemTotal)
	env.SetInt64("Pods", info.Pods)
	env.Set("Operating System", info.OperatingSystem)

	for _, driverStatus := range info.Dstatus {
		status = append(status, [2]string{driverStatus.Name, driverStatus.Status})
	}
	env.SetJson("DriverStatus", status)

	if info.Name != "" {
		env.SetJson("Name", info.Name)
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
