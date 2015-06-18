package hypervisor

import (
	"sync"
	"hyper/lib/glog"
	"hyper/types"
)

type BootConfig struct {
	CPU    int
	Memory int
	Kernel string
	Initrd string
	Bios   string
	Cbfs   string
}

func (ctx *VmContext) loop() {
	for ctx.handler != nil {
		ev, ok := <-ctx.hub
		if !ok {
			glog.Error("hub chan has already been closed")
			break
		} else if ev == nil {
			glog.V(1).Info("got nil event.")
			continue
		}
		glog.V(1).Infof("main event loop got message %d(%s)", ev.Event(), EventString(ev.Event()))
		ctx.handler(ctx, ev)
	}
}

func QemuLoop(vmId string, hub chan QemuEvent, client chan *types.QemuResponse, boot *BootConfig) {

	context, err := initContext(vmId, hub, client, boot)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	//launch routines
	go qmpHandler(context)
	go waitInitReady(context)
	go launchQemu(context)
	go waitPts(context)

	context.loop()
}

func QemuAssociate(vmId string, hub chan QemuEvent, client chan *types.QemuResponse,
		   wg *sync.WaitGroup, pack []byte) {

	if glog.V(1) {
		glog.Infof("VM %s trying to reload with serialized data: %s", vmId, string(pack))
	}

	pinfo, err := vmDeserialize(pack)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	if pinfo.Id != vmId {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: "VM ID mismatch",
		}
		return
	}

	context, err := pinfo.vmContext(hub, client, wg)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	go qmpHandler(context)
	go associateQemu(context)
	go waitPts(context)
	go connectToInit(context)

	context.Become(stateRunning, "RUNNING")

	context.loop()
}
