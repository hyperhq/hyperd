package hypervisor

import (
	"github.com/hyperhq/runv/hypervisor/types"
)

// reportVmShutdown() send report to daemon, notify about that:
//    1. Vm has been shutdown
func (ctx *VmContext) reportVmShutdown() {
	defer func() {
		err := recover()
		if err != nil {
			ctx.Log(WARNING, "panic during send shutdown message to channel")
		}
	}()
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_VM_SHUTDOWN,
		Cause: "VM shut down",
	}
}

func (ctx *VmContext) reportProcessFinished(code int, result *types.ProcessFinished) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  code,
		Cause: "container finished",
		Data:  result,
	}
}

func (ctx *VmContext) reportSuccess(msg string, data interface{}) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_OK,
		Cause: msg,
		Data:  data,
	}
}

// reportUnexpectedRequest send report to daemon, notify about that:
//   1. unexpected event in current state
func (ctx *VmContext) reportUnexpectedRequest(ev VmEvent, state string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_UNEXPECTED,
		Reply: ev,
		Cause: "unexpected event during " + state,
	}
}

// reportVmFault send report to daemon, notify about that:
//   1. vm op failed due to some reason described in `cause`
func (ctx *VmContext) reportVmFault(cause string) {
	defer func() {
		err := recover()
		if err != nil {
			ctx.Log(WARNING, "panic during send vm fault message to channel")
		}
	}()

	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_FAILED,
		Cause: cause,
	}
}
