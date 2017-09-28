package errors

import (
	"net/http"

	"github.com/docker/distribution/registry/api/errcode"
)

const errGroup = "hyperd"

var (
	ErrorCodeCommon = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "COMMONERROR",
		Message:        "%v",
		HTTPStatusCode: http.StatusInternalServerError,
	})

	ErrPodNotFound = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_POD_NOT_FOUND",
		Message:        "Pod %s not found",
		HTTPStatusCode: http.StatusNotFound,
	})

	ErrBadJsonFormat = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_BAD_JSON_FORMAT",
		Message:        "failed to parse json: %v",
		HTTPStatusCode: http.StatusBadRequest,
	})

	ErrSandboxNotExist = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_SANDBOX_NOT_EXIST",
		Message:        "sandbox does not exist",
		HTTPStatusCode: http.StatusPreconditionFailed,
	})

	ErrPodNotAlive = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_POD_NOT_ALIVE",
		Message:        "cannot complete the operation, because the pod %s is not alive",
		HTTPStatusCode: http.StatusPreconditionFailed,
	})

	ErrPodNotRunning = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_POD_NOT_RUNNING",
		Message:        "cannot complete the operation, because the pod %s is not running",
		HTTPStatusCode: http.StatusPreconditionFailed,
	})

	ErrContainerAlreadyRunning = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "HYPER_CONTAINER_RUNNING",
		Message:        "container %s is in running state",
		HTTPStatusCode: http.StatusPreconditionFailed,
	})
)
