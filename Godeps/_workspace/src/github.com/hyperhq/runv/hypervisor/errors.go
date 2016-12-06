package hypervisor

const (
	ET_SPEC      string = "SPEC_ERROR"
	ET_BUSY      string = "RESOURSE_UNAVAILABLE"
	ET_DEVICE    string = "DEVICE_OPERATION_FAIL"
	ET_NOT_READY string = "VM_NOT_READY"
)

type Errors interface {
	Type() string
}

//implement error, hypervisor.Error, and api.Result
type CommonError struct {
	errType   string
	contextId string
	cause     string
}

func (err *CommonError) Error() string {
	return err.cause
}

func (err *CommonError) Type() string {
	return err.errType
}

func (err *CommonError) ResultId() string {
	return err.contextId
}

func (err *CommonError) IsSuccess() bool {
	return false
}

func (err *CommonError) Message() string {
	return err.cause
}

// Error in spec, which is either mistake format or content inconsistency, and
// is checked when elements are being added to Sandbox.
func NewSpecError(id, cause string) *CommonError {
	return &CommonError{
		errType:   ET_SPEC,
		contextId: id,
		cause:     "spec error: " + cause,
	}
}

func NewBusyError(id, cause string) *CommonError {
	return &CommonError{
		errType:   ET_BUSY,
		contextId: id,
		cause:     "resouse unavailable: " + cause,
	}
}

func NewDeviceError(id, cause string) *CommonError {
	return &CommonError{
		errType:   ET_DEVICE,
		contextId: id,
		cause:     "device operation error: " + cause,
	}
}

func NewNotReadyError(id string) *CommonError {
	return &CommonError{
		errType:   ET_NOT_READY,
		contextId: id,
		cause:     "vm is not ready to accept requests",
	}
}
