package pod

import (
	"io"

	"github.com/hyperhq/hyperd/engine"
)

// Backend is the methods that need to be implemented to provide
// system specific functionality.
type Backend interface {
	CmdGetPodInfo(podName string) (interface{}, error)
	CmdGetPodStats(podId string) (interface{}, error)
	CmdCreatePod(podArgs string, autoremove bool) (*engine.Env, error)
	CmdSetPodLabels(podId string, override bool, labels map[string]string) (*engine.Env, error)
	CmdStartPod(in io.ReadCloser, out io.WriteCloser, podId, vmId string, attach bool) (*engine.Env, error)
	CmdPausePod(podId string) error
	CmdUnpausePod(podId string) error
	CmdList(item, podId, vmId string, auxiliary bool) (*engine.Env, error)
	CmdStopPod(podId, stopVm string) (*engine.Env, error)
	CmdKillPod(podName, container string, signal int64) (*engine.Env, error)
	CmdCleanPod(podId string) (*engine.Env, error)
	CmdCreateVm(cpu, mem int, async bool) (*engine.Env, error)
	CmdKillVm(vmId string) (*engine.Env, error)
}
