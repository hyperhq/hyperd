package service

import (
	"github.com/hyperhq/hyperd/engine"
	apitypes "github.com/hyperhq/hyperd/types"
)

// Backend is the methods that need to be implemented to provide
// system specific functionality.
type Backend interface {
	CmdGetServices(podId string) ([]*apitypes.UserService, error)
	CmdAddService(podId, data string) (*engine.Env, error)
	CmdUpdateService(podId, services string) (*engine.Env, error)
	CmdDeleteService(podId, services string) (*engine.Env, error)
}
