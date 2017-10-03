package libhyperstart

import (
	"syscall"

	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
)

type InfUpdateType uint64

const (
	AddInf InfUpdateType = 1 << iota
	DelInf
	AddIP
	DelIP
	SetMtu
)

// Hyperstart interface to hyperstart API
type Hyperstart interface {
	Close()
	LastStreamSeq() uint64

	PauseSync() error
	Unpause() error

	APIVersion() (uint32, error)
	NewContainer(c *hyperstartapi.Container) error
	RestoreContainer(c *hyperstartapi.Container) error
	AddProcess(container string, p *hyperstartapi.Process) error
	SignalProcess(container, process string, signal syscall.Signal) error
	WaitProcess(container, process string) int

	WriteStdin(container, process string, data []byte) (int, error)
	ReadStdout(container, process string, data []byte) (int, error)
	ReadStderr(container, process string, data []byte) (int, error)
	CloseStdin(container, process string) error
	TtyWinResize(container, process string, row, col uint16) error

	StartSandbox(pod *hyperstartapi.Pod) error
	DestroySandbox() error
	WriteFile(container, path string, data []byte) error
	ReadFile(container, path string) ([]byte, error)
	AddRoute(r []hyperstartapi.Route) error
	UpdateInterface(t InfUpdateType, dev, newName string, addresses []hyperstartapi.IpAddress, mtu uint64) error
	OnlineCpuMem() error
}

var NewHyperstart = NewJsonBasedHyperstart
