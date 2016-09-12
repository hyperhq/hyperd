package hypervisor

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/types"
)

func LazyVmLoop(vmId string, hub chan VmEvent, client chan *types.VmResponse, boot *BootConfig) {

	glog.V(1).Infof("Start VM %s in lazy mode, not started yet actually", vmId)

	context, err := InitContext(vmId, hub, client, nil, boot)
	if err != nil {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	if _, ok := context.DCtx.(LazyDriverContext); !ok {
		glog.Error("not a lazy driver, cannot call lazy loop")
		context.reportBadRequest("not a lazy driver, cannot call lazy loop")
		return
	}

	err = context.DCtx.(LazyDriverContext).InitVM(context)
	if err != nil {
		estr := fmt.Sprintf("failed to create VM(%s): %s", vmId, err.Error())
		glog.Error(estr)
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: estr,
		}
		return
	}
	context.Become(statePreparing, StatePreparing)

	context.loop()
}

func statePreparing(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case EVENT_VM_EXIT, ERROR_INTERRUPTED:
		glog.Info("VM exited before start...")
	case COMMAND_SHUTDOWN, COMMAND_RELEASE:
		glog.Info("got shutdown or release command, not started yet")
		ctx.reportVmShutdown()
		ctx.Become(nil, StateNone)
	case COMMAND_WINDOWSIZE:
		cmd := ev.(*WindowSizeCommand)
		ctx.setWindowSize(cmd.ContainerId, cmd.ExecId, cmd.Size)
	case COMMAND_RUN_POD, COMMAND_REPLACE_POD:
		glog.Info("got spec, prepare devices")
		if ok := ctx.lazyPrepareDevice(ev.(*RunPodCommand)); ok {
			ctx.startSocks()
			ctx.DCtx.(LazyDriverContext).LazyLaunch(ctx)
			ctx.setTimeout(60)
			ctx.Become(stateStarting, StateStarting)
		} else {
			glog.Warning("Fail to prepare devices, quit")
			ctx.Become(nil, StateNone)
		}
	case GENERIC_OPERATION:
		ctx.handleGenericOperation(ev.(*GenericOperation))
	default:
		unexpectedEventHandler(ctx, ev, "pod initiating")
	}
}

func (ctx *VmContext) lazyPrepareDevice(cmd *RunPodCommand) bool {

	if len(cmd.Spec.Containers) != len(cmd.Containers) {
		ctx.reportBadRequest("Spec and Container Info mismatch")
		return false
	}

	ctx.InitDeviceContext(cmd.Spec, cmd.Wg, cmd.Containers, cmd.Volumes)

	if glog.V(2) {
		res, _ := json.MarshalIndent(*ctx.vmSpec, "    ", "    ")
		glog.Info("initial vm spec: ", string(res))
	}

	err := ctx.lazyAllocateNetworks()
	if err != nil {
		ctx.reportVmFault(err.Error())
		return false
	}
	ctx.lazyAddBlockDevices()

	return true
}

func (ctx *VmContext) lazyAllocateNetworks() error {
	for i := range ctx.progress.adding.networks {
		name := fmt.Sprintf("eth%d", i)
		addr := ctx.nextPciAddr()
		nic, err := ctx.allocateInterface(i, addr, name)
		if err != nil {
			return err
		}
		ctx.interfaceCreated(nic, true, ctx.Hub)
	}

	return nil
}

func (ctx *VmContext) lazyAddBlockDevices() {
	for blk := range ctx.progress.adding.blockdevs {
		if info, ok := ctx.devices.volumeMap[blk]; ok {
			sid := ctx.nextScsiId()
			ctx.DCtx.(LazyDriverContext).LazyAddDisk(ctx, info.info.Name, "volume", info.info.Filename, info.info.Format, sid)
		} else if info, ok := ctx.devices.imageMap[blk]; ok {
			sid := ctx.nextScsiId()
			ctx.DCtx.(LazyDriverContext).LazyAddDisk(ctx, info.info.Name, "image", info.info.Filename, info.info.Format, sid)
		} else {
			continue
		}
	}
}
