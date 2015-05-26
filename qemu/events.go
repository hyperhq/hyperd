package qemu

import (
    "net"
    "hyper/pod"
    "os"
)

type QemuEvent interface {
    Event() int
}

type QemuExitEvent struct {
    message string
}

type QemuKilledEvent struct {
    success bool
}

type QemuTimeout struct {}

type InitFailedEvent struct {
    reason string
}

type InitConnectedEvent struct {
    conn *net.UnixConn
}

type RunPodCommand struct {
    Spec *pod.UserPod
    Containers []*ContainerInfo
    Volumes    []*VolumeInfo
}

type ReplacePodCommand RunPodCommand

type ExecCommand struct {
    Container string `json:"container,omitempty"`
    Sequence  uint64 `json:"seq"`
    Command []string `json:"cmd"`
    Streams   *TtyIO `json:"-"`
}

type StopPodCommand struct {}
type ShutdownCommand struct {}
type ReleaseVMCommand struct {}

type AttachCommand struct {
    Container string
    Streams   *TtyIO
    Size      *WindowSize
}

type CommandAck struct {
    reply   uint32
    msg     []byte
}

type CommandError struct {
    context *DecodedMessage
    msg     []byte
}

type WindowSizeCommand struct {
    ClientTag   string
    Size        *WindowSize
}

type ContainerCreatedEvent struct {
    Index   int
    Id      string
    Rootfs  string
    Image   string          // if fstype is `dir`, this should be a path relative to share_dir
    // which described the mounted aufs or overlayfs dir.
    Fstype  string
    Workdir string
    Entrypoint []string
    Cmd     []string
    Envs    map[string]string
}

type ContainerInfo struct {
    Id      string
    Rootfs  string
    Image   string          // if fstype is `dir`, this should be a path relative to share_dir
    // which described the mounted aufs or overlayfs dir.
    Fstype  string
    Workdir string
    Entrypoint []string
    Cmd     []string
    Envs    map[string]string
}

type ContainerUnmounted struct {
    Index   int
    Success bool
}

type VolumeReadyEvent struct {
    Name        string      //volumen name in spec
    Filepath    string      //block dev absolute path, or dir path relative to share dir
    Fstype      string      //"xfs", "ext4" etc. for block dev, or "dir" for dir path
    Format      string      //"raw" (or "qcow2") for volume, no meaning for dir path
}

type VolumeInfo struct {
    Name        string      //volumen name in spec
    Filepath    string      //block dev absolute path, or dir path relative to share dir
    Fstype      string      //"xfs", "ext4" etc. for block dev, or "dir" for dir path
    Format      string      //"raw" (or "qcow2") for volume, no meaning for dir path
}

type VolumeUnmounted struct {
    Name        string
    Success     bool
}

type BlockdevInsertedEvent struct {
    Name        string
    SourceType  string //image or volume
    DeviceName  string
    ScsiId      int
}

type BlockdevRemovedEvent struct {
    Name        string
    Success     bool
}

type InterfaceCreated struct {
    Index       int
    PCIAddr     int
    Fd          *os.File
    DeviceName  string
    MacAddr     string
    IpAddr      string
    NetMask     string
    RouteTable  []*RouteRule
}

type InterfaceReleased struct {
    Index       int
    Success     bool
}

type RouteRule struct {
    Destination string
    Gateway     string
    ViaThis     bool
}

type NetDevInsertedEvent struct {
    Index       int
    DeviceName  string
    Address     int
}

type NetDevRemovedEvent struct {
    Index       int
}

type DeviceFailed struct {
    session     QemuEvent
}

type Interrupted struct {
    reason      string
}

func (qe* QemuExitEvent)            Event() int { return EVENT_QEMU_EXIT }
func (qe* QemuKilledEvent)          Event() int { return EVENT_QEMU_KILL }
func (qe* QemuTimeout)              Event() int { return EVENT_QEMU_TIMEOUT }
func (qe* InitConnectedEvent)       Event() int { return EVENT_INIT_CONNECTED }
func (qe* ContainerCreatedEvent)    Event() int { return EVENT_CONTAINER_ADD }
func (qe* ContainerUnmounted)       Event() int { return EVENT_CONTAINER_DELETE }
func (qe* VolumeUnmounted)          Event() int { return EVENT_BLOCK_EJECTED }
func (qe* VolumeReadyEvent)         Event() int { return EVENT_VOLUME_ADD }
func (qe* BlockdevInsertedEvent)    Event() int { return EVENT_BLOCK_INSERTED }
func (qe* BlockdevRemovedEvent)     Event() int { return EVENT_VOLUME_DELETE }
func (qe* InterfaceCreated)         Event() int { return EVENT_INTERFACE_ADD }
func (qe* InterfaceReleased)        Event() int { return EVENT_INTERFACE_DELETE }
func (qe* NetDevInsertedEvent)      Event() int { return EVENT_INTERFACE_INSERTED }
func (qe* NetDevRemovedEvent)       Event() int { return EVENT_INTERFACE_EJECTED }
func (qe* RunPodCommand)            Event() int { return COMMAND_RUN_POD }
func (qe* StopPodCommand)           Event() int { return COMMAND_STOP_POD }
func (qe* ReplacePodCommand)        Event() int { return COMMAND_REPLACE_POD }
func (qe* ExecCommand)              Event() int { return COMMAND_EXEC }
func (qe* AttachCommand)            Event() int { return COMMAND_ATTACH }
func (qe* WindowSizeCommand)        Event() int { return COMMAND_WINDOWSIZE }
func (qe* ShutdownCommand)          Event() int { return COMMAND_SHUTDOWN }
func (qe* ReleaseVMCommand)         Event() int { return COMMAND_RELEASE }
func (qe* CommandAck)               Event() int { return COMMAND_ACK }
func (qe* InitFailedEvent)          Event() int { return ERROR_INIT_FAIL }
func (qe* DeviceFailed)             Event() int { return ERROR_QMP_FAIL }
func (qe* Interrupted)              Event() int { return ERROR_INTERRUPTED }
func (qe* CommandError)             Event() int { return ERROR_CMD_FAIL }
