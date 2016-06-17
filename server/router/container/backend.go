package container

import (
	"io"

	"github.com/docker/engine-api/types"
	"github.com/hyperhq/hyperd/daemon"
	"github.com/hyperhq/hyperd/engine"
)

type Backend interface {
	CmdGetContainerInfo(container string) (interface{}, error)
	CmdGetContainerLogs(name string, c *daemon.ContainerLogsConfig) error
	CmdExitCode(container, tag string) (int, error)
	CmdCreateContainer(podId string, containerArgs []byte) (string, error)
	CmdKillContainer(name string, sig int64) (*engine.Env, error)
	CmdStopContainer(name string) (*engine.Env, error)
	CmdContainerRename(oldName, newName string) (*engine.Env, error)
	CmdExec(in io.ReadCloser, out io.WriteCloser, key, id, cmd, tag string, terminal bool) error
	CmdAttach(in io.ReadCloser, out io.WriteCloser, key, id, tag string) error
	CmdCommitImage(name string, cfg *types.ContainerCommitConfig) (*engine.Env, error)
	CmdTtyResize(podId, tag string, h, w int) error
}
