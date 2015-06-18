package hypervisor

import (
	"hyper/types"
)

// reportVmRun() send report to daemon, notify about that:
//    1. Vm has been running.
//    2. Init is ready for accepting commands
func (ctx *VmContext) reportVmRun() {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_VM_RUNNING,
		Cause: "Vm runs",
	}
}

// reportVmShutdown() send report to daemon, notify about that:
//    1. Vm has been shutdown
func (ctx *VmContext) reportVmShutdown() {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_VM_SHUTDOWN,
		Cause: "qemu shut down",
	}
}

func (ctx *VmContext) reportPodRunning(msg string, data interface{}) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_RUNNING,
		Cause: msg,
		Data:  data,
	}
}

func (ctx *VmContext) reportPodStopped() {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_STOPPED,
		Cause: "All device detached successful",
	}
}

func (ctx *VmContext) reportPodFinished(result *PodFinished) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_FINISHED,
		Cause: "POD run finished",
		Data:  result.result,
	}
}

func (ctx *VmContext) reportSuccess(msg string, data interface{}) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_OK,
		Cause: msg,
		Data:  data,
	}
}

func (ctx *VmContext) reportBusy(msg string) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_BUSY,
		Cause: msg,
	}
}

// reportBadRequest send report to daemon, notify about that:
//   1. anything wrong in the request, such as json format, slice length, etc.
func (ctx *VmContext) reportBadRequest(cause string) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_BAD_REQUEST,
		Cause: cause,
	}
}

// reportVmFault send report to daemon, notify about that:
//   1. vm op failed due to some reason described in `cause`
func (ctx *VmContext) reportVmFault(cause string) {
	ctx.client <- &types.QemuResponse{
		VmId:  ctx.Id,
		Code:  types.E_FAILED,
		Cause: cause,
	}
}
