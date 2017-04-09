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
)
