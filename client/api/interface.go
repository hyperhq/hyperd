package api

import (
	"io"

	"github.com/hyperhq/hyperd/engine"
	"github.com/hyperhq/hyperd/types"

	dockertypes "github.com/docker/engine-api/types"
)

type APIInterface interface {
	Login(auth dockertypes.AuthConfig, response *dockertypes.AuthResponse) (remove bool, err error)

	Attach(container string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error
	CreateExec(containerId string, command []byte, tty bool) (string, error)
	StartExec(containerId, execId string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) error
	ExecVM(podID string, command []byte, stdin io.ReadCloser, stdout, stderr io.Writer) error

	WinResize(id, tag string, height, width int) error

	List(item, pod, vm string) (*engine.Env, error)
	CreateContainer(podID string, spec interface{}) (string, int, error)
	StartContainer(container string) error
	GetContainerInfo(container string) (*types.ContainerInfo, error)
	GetContainerByPod(podId string) (string, error)
	GetExitCode(container, tag string, wait bool) error
	ContainerLogs(container, since string, timestamp, follow, stdout, stderr bool, tail string) (io.ReadCloser, string, error)
	KillContainer(container string, sig int) error
	StopContainer(container string) error
	RemoveContainer(container string) error

	GetPodInfo(podName string) (*types.PodInfo, error)
	CreatePod(spec interface{}) (string, int, error)
	StartPod(podId string) error
	StopPod(podId, stopVm string) (int, string, error)
	RmPod(id string) error
	PausePod(podId string) error
	UnpausePod(podId string) error
	KillPod(pod string, sig int) error

	// PortMapping APIs
	ListPortMappings(podId string) ([]*types.PortMapping, error)
	AddPortMappings(podId string, pms []*types.PortMapping) error
	DeletePortMappings(podId string, pms []*types.PortMapping) error

	Build(name string, hasBody bool, body io.Reader) (io.ReadCloser, string, error)
	Commit(container, repo, author, message string, changes []string, pause bool) (string, error)
	Load(body io.Reader, name string, refs map[string]string) (io.ReadCloser, string, error)
	Save(imageIDs []string, format string, refs map[string]string) (io.ReadCloser, error)
	GetImages(all, quiet bool) (*engine.Env, error)
	RemoveImage(image string, noprune, force bool) (*engine.Env, error)
	Pull(image string, authConfig dockertypes.AuthConfig) (io.ReadCloser, string, int, error)
	Push(tag, repo string, authConfig dockertypes.AuthConfig) (io.ReadCloser, string, int, error)

	CreateVm(cpu, mem int, async bool) (id string, err error)
	RmVm(vm string) (err error)

	Info() (*engine.Env, error)
}
