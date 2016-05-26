package system

import (
	"github.com/docker/engine-api/types"
	"github.com/hyperhq/hyperd/engine"
	apitypes "github.com/hyperhq/hyperd/types"
)

// Backend is the methods that need to be implemented to provide
// system specific functionality.
type Backend interface {
	CmdSystemInfo() (*apitypes.InfoResponse, error)
	CmdSystemVersion() *engine.Env
	CmdAuthenticateToRegistry(authConfig *types.AuthConfig) (string, error)
}
