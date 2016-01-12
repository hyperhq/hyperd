package docker

import (
	"encoding/json"
	"io"

	"github.com/docker/docker/cliconfig"
)

func (cli Docker) SendCmdAuth(body io.ReadCloser) (string, error) {
	var config *cliconfig.AuthConfig
	err := json.NewDecoder(body).Decode(&config)
	body.Close()
	if err != nil {
		return "", err
	}
	return cli.daemon.RegistryService.Auth(config)
}
