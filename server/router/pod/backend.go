package pod

import (
	"github.com/hyperhq/hyperd/engine"
)

// Backend is the methods that need to be implemented to provide
// system specific functionality.
type Backend interface {
	CmdGetPodInfo(podName string) (interface{}, error)
	CmdGetPodStats(podId string) (interface{}, error)
	CmdCreatePod(podArgs string) (*engine.Env, error)
	CmdSetPodLabels(podId string, override bool, labels map[string]string) (*engine.Env, error)
	CmdStartPod(podId string) (*engine.Env, error)
	CmdPausePod(podId string) error
	CmdUnpausePod(podId string) error
	CmdList(item, podId, vmId string) (*engine.Env, error)
	CmdStopPod(podId, stopVm string) (*engine.Env, error)
	CmdKillPod(podName, container string, signal int64) (*engine.Env, error)
	CmdCleanPod(podId string) (*engine.Env, error)

	//port mapping
	CmdListPortMappings(podId string) (*engine.Env, error)
	CmdAddPortMappings(podId string, pms []byte) (*engine.Env, error)
	CmdDeletePortMappings(podId string, pms []byte) (*engine.Env, error)
}
