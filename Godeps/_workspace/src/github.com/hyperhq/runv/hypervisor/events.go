package hypervisor

import (
	"net"
	"os"
)

type VmEvent interface {
	Event() int
}

type VmExit struct{}

type VmStartFailEvent struct {
	Message string
}

type VmKilledEvent struct {
	Success bool
}

type PodFinished struct {
	result []uint32
}

type VmTimeout struct{}

type InitFailedEvent struct {
	Reason string
}

type InitConnectedEvent struct {
	conn *net.UnixConn
}

type GetPodStatsCommand struct {
	Id string
}

type OnlineCpuMemCommand struct{}

type ShutdownCommand struct {
	Wait bool
}
type ReleaseVMCommand struct{}

type AttachCommand struct {
	Streams   *TtyIO
	Size      *WindowSize
	Container string
}

type CommandAck struct {
	reply *hyperstartCmd
	msg   []byte
}

type CommandError CommandAck

type WindowSizeCommand struct {
	ContainerId string
	ExecId      string
	Size        *WindowSize
}

type ContainerCreatedEvent struct {
	Index  int
	Id     string
	Rootfs string
	Image  string // if fstype is `dir`, this should be a path relative to share_dir
	// which described the mounted aufs or overlayfs dir.
	Fstype     string
	Workdir    string
	Entrypoint []string
	Cmd        []string
	Envs       map[string]string
}

type ContainerUnmounted struct {
	Index   int
	Success bool
}

type VolumeReadyEvent struct {
	Name     string //volumen name in spec
	Filepath string //block dev absolute path, or dir path relative to share dir
	Fstype   string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format   string //"raw" (or "qcow2") for volume, no meaning for dir path
}

type VolumeInfo struct {
	Name         string //volumen name in spec
	Filepath     string //block dev absolute path, or dir path relative to share dir
	Fstype       string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format       string //"raw" (or "qcow2") for volume, no meaning for dir path
	DockerVolume bool
}

type VolumeUnmounted struct {
	Name    string
	Success bool
}

type BlockdevInsertedEvent struct {
	Name       string
	SourceType string //image or volume
	DeviceName string
	ScsiId     int
	ScsiAddr   string // pass scsi addr to hyperstart
}

type DevSkipEvent struct{}

type InterfaceCreated struct {
	Id         string //user specified in (ref api.InterfaceDescription: a user identifier of interface, user may use this to specify a nic, normally you can use IPAddr as an Id, however, in some driver (probably vbox?), user may not specify the IPAddr.)
	Index      int
	PCIAddr    int
	Fd         *os.File
	Bridge     string
	HostDevice string
	DeviceName string
	MacAddr    string
	IpAddr     string
	NetMask    string
	RouteTable []*RouteRule
}

type InterfaceReleased struct {
	Index   int
	Success bool
}

type RouteRule struct {
	Destination string
	Gateway     string
	ViaThis     bool
}

type NetDevInsertedEvent struct {
	Id         string
	Index      int
	DeviceName string
	Address    int
}

func (ne *NetDevInsertedEvent) ResultId() string {
	return ne.Id
}

func (ne *NetDevInsertedEvent) IsSuccess() bool {
	return true
}

func (ne *NetDevInsertedEvent) Message() string {
	return "NIC inserted"
}

type NetDevRemovedEvent struct {
	Index int
}

type DeviceFailed struct {
	Session VmEvent
}

//Device Failed as api.Result
func (df *DeviceFailed) ResultId() string {
	switch s := df.Session.(type) {
	case *InterfaceCreated:
		return s.Id
	case *NetDevInsertedEvent:
		return s.Id
	default:
		return ""
	}
}

func (df *DeviceFailed) IsSuccess() bool {
	return false
}

func (df *DeviceFailed) Message() string {
	return "Device operation failed"
}

type Interrupted struct {
	Reason string
}

type GenericOperation struct {
	OpName string
	State  []string
	OpFunc func(ctx *VmContext, result chan<- error)
	Result chan<- error
}

func (qe *VmStartFailEvent) Event() int      { return ERROR_VM_START_FAILED }
func (qe *VmExit) Event() int                { return EVENT_VM_EXIT }
func (qe *VmKilledEvent) Event() int         { return EVENT_VM_KILL }
func (qe *VmTimeout) Event() int             { return EVENT_VM_TIMEOUT }
func (qe *PodFinished) Event() int           { return EVENT_POD_FINISH }
func (qe *InitConnectedEvent) Event() int    { return EVENT_INIT_CONNECTED }
func (qe *ContainerCreatedEvent) Event() int { return EVENT_CONTAINER_ADD }
func (qe *ContainerUnmounted) Event() int    { return EVENT_CONTAINER_DELETE }
func (qe *VolumeUnmounted) Event() int       { return EVENT_BLOCK_EJECTED }
func (qe *VolumeReadyEvent) Event() int      { return EVENT_VOLUME_ADD }
func (qe *BlockdevInsertedEvent) Event() int { return EVENT_BLOCK_INSERTED }
func (qe *DevSkipEvent) Event() int          { return EVENT_DEV_SKIP }
func (qe *InterfaceCreated) Event() int      { return EVENT_INTERFACE_ADD }
func (qe *InterfaceReleased) Event() int     { return EVENT_INTERFACE_DELETE }
func (qe *NetDevInsertedEvent) Event() int   { return EVENT_INTERFACE_INSERTED }
func (qe *NetDevRemovedEvent) Event() int    { return EVENT_INTERFACE_EJECTED }
func (qe *GetPodStatsCommand) Event() int    { return COMMAND_GET_POD_STATS }
func (qe *OnlineCpuMemCommand) Event() int   { return COMMAND_ONLINECPUMEM }
func (qe *AttachCommand) Event() int         { return COMMAND_ATTACH }
func (qe *WindowSizeCommand) Event() int     { return COMMAND_WINDOWSIZE }
func (qe *ShutdownCommand) Event() int       { return COMMAND_SHUTDOWN }
func (qe *ReleaseVMCommand) Event() int      { return COMMAND_RELEASE }
func (qe *CommandAck) Event() int            { return COMMAND_ACK }
func (qe *GenericOperation) Event() int      { return GENERIC_OPERATION }
func (qe *InitFailedEvent) Event() int       { return ERROR_INIT_FAIL }
func (qe *DeviceFailed) Event() int          { return ERROR_QMP_FAIL }
func (qe *Interrupted) Event() int           { return ERROR_INTERRUPTED }
func (qe *CommandError) Event() int          { return ERROR_CMD_FAIL }
