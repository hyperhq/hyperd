package hypervisor

const (
	BaseDir         = "/var/run/hyper"
	HyperSockName   = "hyper.sock"
	TtySockName     = "tty.sock"
	ConsoleSockName = "console.sock"
	ShareDirTag     = "share_dir"
	DefaultKernel   = "/var/lib/hyper/kernel"
	DefaultInitrd   = "/var/lib/hyper/hyper-initrd.img"
	DetachKeys      = "ctrl-p,ctrl-q"

	// cpu/mem hotplug constants
	DefaultMaxCpus = 8     // CONFIG_NR_CPUS hyperstart.git/build/kernel_config
	DefaultMaxMem  = 32768 // size in MiB
)

var InterfaceCount int = 1
var PciAddrFrom int = 0x05

const (
	ST_CREATING = iota
	ST_CREATED
	ST_STARTING
	ST_RUNNING
	ST_STOPPING
)

const (
	EVENT_VM_EXIT = iota
	EVENT_VM_KILL
	EVENT_VM_TIMEOUT
	// TODO EVENT_BLOCK_EJECTED EVENT_BLOCK_INSERTED EVENT_INTERFACE_ADD EVENT_INTERFACE_INSERTED EVENT_INTERFACE_EJECTED are not referenced expect in events.go
	EVENT_BLOCK_INSERTED
	EVENT_BLOCK_EJECTED
	EVENT_INTERFACE_ADD
	EVENT_INTERFACE_INSERTED
	EVENT_INTERFACE_EJECTED
	COMMAND_SHUTDOWN
	COMMAND_RELEASE
	COMMAND_ATTACH
	COMMAND_PAUSEVM
	ERROR_VM_START_FAILED
	ERROR_INIT_FAIL
	ERROR_QMP_FAIL
	ERROR_INTERRUPTED
)

func EventString(ev int) string {
	switch ev {
	case ERROR_VM_START_FAILED:
		return "ERROR_VM_START_FAILED"
	case EVENT_VM_EXIT:
		return "EVENT_VM_EXIT"
	case EVENT_VM_KILL:
		return "EVENT_VM_KILL"
	case EVENT_VM_TIMEOUT:
		return "EVENT_VM_TIMEOUT"
	case COMMAND_PAUSEVM:
		return "COMMAND_PAUSEVM"
	case COMMAND_SHUTDOWN:
		return "COMMAND_SHUTDOWN"
	case COMMAND_RELEASE:
		return "COMMAND_RELEASE"
	case COMMAND_ATTACH:
		return "COMMAND_ATTACH"
	case ERROR_INIT_FAIL:
		return "ERROR_INIT_FAIL"
	case ERROR_QMP_FAIL:
		return "ERROR_QMP_FAIL"
	case ERROR_INTERRUPTED:
		return "ERROR_INTERRUPTED"
	}
	return "UNKNOWN"
}
