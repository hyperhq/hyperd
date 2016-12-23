package types

const (
	E_OK = iota
	E_VM_RUNNING
	E_VM_SHUTDOWN
	E_POD_RUNNING
	E_BAD_REQUEST
	E_FAILED
	E_EXEC_FINISHED
	E_CONTAINER_FINISHED
	E_BUSY
	E_NO_TTY
	E_JSON_PARSE_FAIL
	E_UNEXPECTED
)

// status for POD or container
const (
	S_POD_NONE = iota
	S_POD_CREATED
	S_POD_RUNNING
	S_POD_FAILED
	S_POD_SUCCEEDED
	S_POD_PAUSED

	S_VM_IDLE
	S_VM_ASSOCIATED
	S_VM_PAUSED
)

type ProcessFinished struct {
	Id   string
	Code uint8
	Ack  chan bool
}

type VmResponse struct {
	VmId  string
	Code  int
	Cause string
	Reply interface{}
	Data  interface{}
}
