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
	CmdAttach(in io.ReadCloser, out io.WriteCloser, id string) error
	CmdCommitImage(name string, cfg *types.ContainerCommitConfig) (*engine.Env, error)
	CmdTtyResize(podId, tag string, h, w int) error
	CreateExec(id, cmd string, terminal bool) (string, error)
	StartExec(stdin io.ReadCloser, stdout io.WriteCloser, containerId, execId string) error
}
