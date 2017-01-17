package libhyperstart

import (
	"io"
	"syscall"

	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
)

type Hyperstart interface {
	Close()
	LastStreamSeq() uint64

	APIVersion() (uint32, error)
	ProcessAsyncEvents() (<-chan hyperstartapi.ProcessAsyncEvent, error)
	NewContainer(c *hyperstartapi.Container) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error)
	AddProcess(container string, p *hyperstartapi.Process) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error)
	SignalProcess(container, process string, signal syscall.Signal) error
	TtyWinResize(container, process string, row, col uint16) error

	StartSandbox(pod *hyperstartapi.Pod) error
	DestroySandbox() error
	WriteFile(container, path string, data []byte) error
	ReadFile(container, path string) ([]byte, error)
	AddRoute(r []hyperstartapi.Route) error
	UpdateInterface(dev, ip, mask string) error
	OnlineCpuMem() error
}
