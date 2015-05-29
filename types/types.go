package types

const (
    E_OK = iota
    E_VM_RUNNING
    E_VM_SHUTDOWN
    E_POD_RUNNING
    E_POD_STOPPED
    E_POD_FINISHED
    E_BAD_REQUEST
    E_FAILED
    E_EXEC_FINISH
    E_BUSY
    E_NO_TTY
    E_JSON_PARSE_FAIL
)

// status for POD or container
const (
    S_POD_NONE = iota
    S_POD_CREATED
    S_POD_RUNNING
    S_POD_FAILED
    S_POD_SUCCEEDED

    S_VM_IDLE
    S_VM_ASSOCIATED
)

type QemuResponse struct {
    VmId string
    Code int
    Cause string
    Data interface{}
}
