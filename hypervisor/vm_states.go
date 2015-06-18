package hypervisor

import (
	"encoding/json"
	"fmt"
	"hyper/lib/glog"
	"hyper/pod"
	"hyper/types"
	"time"
)

func (ctx *VmContext) timedKill(seconds int) {
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if ctx != nil && ctx.handler != nil {
			ctx.DCtx.Kill(ctx)
		}
	})
}

func (ctx *VmContext) onQemuExit(reclaim bool) bool {
	glog.V(1).Info("qemu has exit...")
	ctx.reportVmShutdown()
	ctx.setTimeout(60)

	ctx.DCtx.Kill(ctx)
	if reclaim {
		ctx.reclaimDevice()
	}

	return ctx.tryClose()
}

func (ctx *VmContext) reclaimDevice() {
	ctx.releaseVolumeDir()
	ctx.releaseOverlayDir()
	ctx.releaseAufsDir()
	ctx.removeDMDevice()
	ctx.releaseNetwork()
}

func (ctx *VmContext) detatchDevice() {
	ctx.releaseVolumeDir()
	ctx.releaseOverlayDir()
	ctx.releaseAufsDir()
	ctx.removeVolumeDrive()
	ctx.removeImageDrive()
	ctx.removeInterface()
}

func (ctx *VmContext) prepareDevice(cmd *RunPodCommand) bool {

	if len(cmd.Spec.Containers) != len(cmd.Containers) {
		ctx.reportBadRequest("Spec and Container Info mismatch")
		return false
	}

	ctx.InitDeviceContext(cmd.Spec, cmd.Wg, cmd.Containers, cmd.Volumes)

	if glog.V(2) {
		res, _ := json.MarshalIndent(*ctx.vmSpec, "    ", "    ")
		glog.Info("initial vm spec: ", string(res))
	}

	ctx.allocateNetworks()
	ctx.addBlockDevices()

	return true
}

func (ctx *VmContext) setWindowSize(tag string, size *WindowSize) {
	if session, ok := ctx.ttySessions[tag]; ok {
		cmd := map[string]interface{}{
			"seq":    session,
			"row":    size.Row,
			"column": size.Column,
		}
		msg, err := json.Marshal(cmd)
		if err != nil {
			ctx.reportBadRequest(fmt.Sprintf("command window size parse failed"))
			return
		}
		ctx.vm <- &DecodedMessage{
			code:    INIT_WINSIZE,
			message: msg,
		}
	} else {
		msg := fmt.Sprintf("cannot resolve client tag %s", tag)
		ctx.reportBadRequest(msg)
		glog.Error(msg)
	}
}

func (ctx *VmContext) execCmd(cmd *ExecCommand) {
	cmd.Sequence = ctx.nextAttachId()
	pkg, err := json.Marshal(*cmd)
	if err != nil {
		cmd.Streams.Callback <- &types.QemuResponse{
			VmId: ctx.Id, Code: types.E_JSON_PARSE_FAIL,
			Cause: fmt.Sprintf("command %s parse failed", cmd.Command), Data: cmd.Sequence,
		}
		return
	}
	ctx.ptys.ptyConnect(ctx, ctx.Lookup(cmd.Container), cmd.Sequence, cmd.Streams)
	ctx.clientReg(cmd.Streams.ClientTag, cmd.Sequence)
	ctx.vm <- &DecodedMessage{
		code:    INIT_EXECCMD,
		message: pkg,
	}
}

func (ctx *VmContext) attachCmd(cmd *AttachCommand) {
	idx := ctx.Lookup(cmd.Container)
	if idx < 0 || idx > len(ctx.vmSpec.Containers) || ctx.vmSpec.Containers[idx].Tty == 0 {
		ctx.reportBadRequest(fmt.Sprintf("tty is not configured for %s", cmd.Container))
		cmd.Streams.Callback <- &types.QemuResponse{
			VmId:  ctx.Id,
			Code:  types.E_NO_TTY,
			Cause: fmt.Sprintf("tty is not configured for %s", cmd.Container),
			Data:  uint64(0),
		}
		return
	}
	session := ctx.vmSpec.Containers[idx].Tty
	glog.V(1).Infof("Connecting tty for %s on session %d", cmd.Container, session)
	ctx.ptys.ptyConnect(ctx, idx, session, cmd.Streams)
	ctx.clientReg(cmd.Streams.ClientTag, session)
	if cmd.Size != nil {
		ctx.setWindowSize(cmd.Streams.ClientTag, cmd.Size)
	}
}

func (ctx *VmContext) startPod() {
	pod, err := json.Marshal(*ctx.vmSpec)
	if err != nil {
		ctx.Hub <- &InitFailedEvent{
			Reason: "Generated wrong run profile " + err.Error(),
		}
		return
	}
	ctx.vm <- &DecodedMessage{
		code:    INIT_STARTPOD,
		message: pod,
	}
}

func (ctx *VmContext) stopPod() {
	ctx.setTimeout(30)
	ctx.vm <- &DecodedMessage{
		code:    INIT_STOPPOD,
		message: []byte{},
	}
}

func (ctx *VmContext) exitVM(err bool, msg string, hasPod bool, wait bool) {
	ctx.wait = wait
	if hasPod {
		ctx.shutdownVM(err, msg)
		ctx.Become(stateTerminating, "TERMINATING")
	} else {
		ctx.poweroffVM(err, msg)
		ctx.Become(stateDestroying, "DESTROYING")
	}
}

func (ctx *VmContext) shutdownVM(err bool, msg string) {
	if err {
		ctx.reportVmFault(msg)
		glog.Error("Shutting down because of an exception: ", msg)
	}
	ctx.setTimeout(10)
	ctx.vm <- &DecodedMessage{code: INIT_DESTROYPOD, message: []byte{}}
}

func (ctx *VmContext) poweroffVM(err bool, msg string) {
	if err {
		ctx.reportVmFault(msg)
		glog.Error("Shutting down because of an exception: ", msg)
	}
	ctx.DCtx.Shutdown(ctx)
	ctx.timedKill(10)
}

// state machine
func commonStateHandler(ctx *VmContext, ev VmEvent, hasPod bool) bool {
	processed := true
	switch ev.Event() {
	case EVENT_VM_EXIT:
		glog.Info("Got VM shutdown event, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onQemuExit(hasPod); !closed {
			ctx.Become(stateDestroying, "DESTROYING")
		}
	case ERROR_INTERRUPTED:
		glog.Info("Connection interrupted, quit...")
		ctx.exitVM(true, "connection to VM broken", hasPod, false)
	case COMMAND_SHUTDOWN:
		glog.Info("got shutdown command, shutting down")
		ctx.exitVM(false, "", hasPod, ev.(*ShutdownCommand).Wait)
	default:
		processed = false
	}
	return processed
}

func deviceInitHandler(ctx *VmContext, ev VmEvent) bool {
	processed := true
	switch ev.Event() {
	case EVENT_BLOCK_INSERTED:
		info := ev.(*BlockdevInsertedEvent)
		ctx.blockdevInserted(info)
	case EVENT_INTERFACE_ADD:
		info := ev.(*InterfaceCreated)
		ctx.interfaceCreated(info)
		h := &HostNicInfo{
			Fd:      uint64(info.Fd.Fd()),
			Device:  info.HostDevice,
			Mac:     info.MacAddr,
			Bridge:  info.Bridge,
			Gateway: info.Bridge,
		}
		g := &GuestNicInfo{
			Device:  info.DeviceName,
			Ipaddr:  info.IpAddr,
			Index:   info.Index,
			Busaddr: info.PCIAddr,
		}
		ctx.DCtx.AddNic(ctx, h, g)
	case EVENT_INTERFACE_INSERTED:
		info := ev.(*NetDevInsertedEvent)
		ctx.netdevInserted(info)
	default:
		processed = false
	}
	return processed
}

func deviceRemoveHandler(ctx *VmContext, ev VmEvent) (bool, bool) {
	processed := true
	success := true
	switch ev.Event() {
	case EVENT_CONTAINER_DELETE:
		success = ctx.onContainerRemoved(ev.(*ContainerUnmounted))
		glog.V(1).Info("Unplug container return with ", success)
	case EVENT_INTERFACE_DELETE:
		success = ctx.onInterfaceRemoved(ev.(*InterfaceReleased))
		glog.V(1).Info("Unplug interface return with ", success)
	case EVENT_BLOCK_EJECTED:
		success = ctx.onVolumeRemoved(ev.(*VolumeUnmounted))
		glog.V(1).Info("Unplug block device return with ", success)
	case EVENT_VOLUME_DELETE:
		success = ctx.onBlockReleased(ev.(*BlockdevRemovedEvent))
		glog.V(1).Info("release volume return with ", success)
	case EVENT_INTERFACE_EJECTED:
		n := ev.(*NetDevRemovedEvent)
		nic := ctx.devices.networkMap[n.Index]
		var maps []pod.UserContainerPort

		for _, c := range ctx.userSpec.Containers {
			for _, m := range c.Ports {
				maps = append(maps, m)
			}
		}

		glog.V(1).Infof("release %d interface: %s", n.Index, nic.IpAddr)
		go ReleaseInterface(n.Index, nic.IpAddr, nic.Fd, maps, ctx.Hub)
	default:
		processed = false
	}
	return processed, success
}

func initFailureHandler(ctx *VmContext, ev VmEvent) bool {
	processed := true
	switch ev.Event() {
	case ERROR_INIT_FAIL: // Qemu connection Failure
		reason := ev.(*InitFailedEvent).Reason
		glog.Error(reason)
	case ERROR_QMP_FAIL: // Device allocate and insert Failure
		reason := "QMP protocol exception"
		if ev.(*DeviceFailed).Session != nil {
			reason = "QMP protocol exception: failed while waiting " + EventString(ev.(*DeviceFailed).Session.Event())
		}
		glog.Error(reason)
	default:
		processed = false
	}
	return processed
}

func stateInit(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, false); processed {
		//processed by common
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.shutdownVM(true, "Fail during init environment")
		ctx.Become(stateDestroying, "DESTROYING")
	} else {
		switch ev.Event() {
		case EVENT_VM_START_FAILED:
			glog.Error("Qemu did not start up properly, go to cleaning up")
			ctx.reportVmFault("Qemu did not start up properly, go to cleaning up")
			ctx.Close()
		case EVENT_INIT_CONNECTED:
			glog.Info("begin to wait vm commands")
			ctx.reportVmRun()
		case COMMAND_RELEASE:
			glog.Info("no pod on vm, got release, quit.")
			ctx.shutdownVM(false, "")
			ctx.Become(stateDestroying, "DESTRYING")
			ctx.reportVmShutdown()
		case COMMAND_EXEC:
			ctx.execCmd(ev.(*ExecCommand))
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			ctx.setWindowSize(cmd.ClientTag, cmd.Size)
		case COMMAND_RUN_POD, COMMAND_REPLACE_POD:
			glog.Info("got spec, prepare devices")
			if ok := ctx.prepareDevice(ev.(*RunPodCommand)); ok {
				ctx.setTimeout(60)
				ctx.Become(stateStarting, "STARTING")
			}
		default:
			glog.Warning("got event during pod initiating")
		}
	}
}

func stateStarting(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, true); processed {
		//processed by common
	} else if processed := deviceInitHandler(ctx, ev); processed {
		if ctx.deviceReady() {
			glog.V(1).Info("device ready, could run pod.")
			ctx.startPod()
		}
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.shutdownVM(true, "Fail during init pod running environment")
		ctx.Become(stateTerminating, "TERMINATING")
	} else {
		switch ev.Event() {
		case EVENT_VM_START_FAILED:
			glog.Info("Qemu did not start up properly, go to cleaning up")
			if closed := ctx.onQemuExit(true); !closed {
				ctx.Become(stateDestroying, "DESTROYING")
			}
		case EVENT_INIT_CONNECTED:
			glog.Info("begin to wait vm commands")
			ctx.reportVmRun()
		case COMMAND_RELEASE:
			glog.Info("pod starting, got release, please wait")
			ctx.reportBusy("")
		case COMMAND_ATTACH:
			ctx.attachCmd(ev.(*AttachCommand))
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			if ctx.userSpec.Tty {
				ctx.setWindowSize(cmd.ClientTag, cmd.Size)
			}
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[starting] got init ack to %d", ack.reply)
			if ack.reply == INIT_STARTPOD {
				ctx.unsetTimeout()
				var pinfo []byte = []byte{}
				persist, err := ctx.dump()
				if err == nil {
					buf, err := persist.serialize()
					if err == nil {
						pinfo = buf
					}
				}
				ctx.reportSuccess("Start POD success", pinfo)
				ctx.Become(stateRunning, "RUNNING")
				glog.Info("pod start success ", string(ack.msg))
			}
		case ERROR_CMD_FAIL:
			ack := ev.(*CommandError)
			if ack.context.code == INIT_STARTPOD {
				reason := "Start POD failed"
				ctx.shutdownVM(true, reason)
				ctx.Become(stateTerminating, "TERMINATING")
				glog.Error(reason)
			}
		case EVENT_VM_TIMEOUT:
			reason := "Start POD timeout"
			ctx.shutdownVM(true, reason)
			ctx.Become(stateTerminating, "TERMINATING")
			glog.Error(reason)
		default:
			glog.Warning("got event during pod initiating")
		}
	}
}

func stateRunning(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, true); processed {
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.shutdownVM(true, "Fail during reconnect to a running pod")
		ctx.Become(stateTerminating, "TERMINATING")
	} else {
		switch ev.Event() {
		case COMMAND_STOP_POD:
			ctx.stopPod()
			ctx.Become(statePodStopping, "STOPPING")
		case COMMAND_RELEASE:
			glog.Info("pod is running, got release command, let qemu fly")
			ctx.Become(nil, "NONE")
			ctx.reportSuccess("", nil)
		case COMMAND_EXEC:
			ctx.execCmd(ev.(*ExecCommand))
		case COMMAND_ATTACH:
			ctx.attachCmd(ev.(*AttachCommand))
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			if ctx.userSpec.Tty {
				ctx.setWindowSize(cmd.ClientTag, cmd.Size)
			}
		case EVENT_POD_FINISH:
			result := ev.(*PodFinished)
			ctx.reportPodFinished(result)
			ctx.shutdownVM(false, "")
			ctx.Become(stateTerminating, "TERMINATING")
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[running] got init ack to %d", ack.reply)
		case ERROR_CMD_FAIL:
			ack := ev.(*CommandError)
			if ack.context.code == INIT_EXECCMD {
				cmd := ExecCommand{}
				json.Unmarshal(ack.context.message, &cmd)
				ctx.ptys.Close(ctx, cmd.Sequence)
				glog.V(0).Infof("Exec command %s on session %d failed", cmd.Command[0], cmd.Sequence)
			}
		default:
			glog.Warning("got unexpected event during pod running")
		}
	}
}

func statePodStopping(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, true); processed {
	} else {
		switch ev.Event() {
		case COMMAND_RELEASE:
			glog.Info("pod stopping, got release, quit.")
			ctx.unsetTimeout()
			ctx.shutdownVM(false, "got release, quit")
			ctx.Become(stateTerminating, "TERMINATING")
			ctx.reportVmShutdown()
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[Stopping] got init ack to %d", ack.reply)
			if ack.reply == INIT_STOPPOD {
				glog.Info("POD stopped ", string(ack.msg))
				ctx.detatchDevice()
				ctx.Become(stateCleaning, "CLEANING")
			}
		case ERROR_CMD_FAIL:
			ack := ev.(*CommandError)
			if ack.context.code == INIT_STOPPOD {
				ctx.unsetTimeout()
				ctx.shutdownVM(true, "Stop pod failed as init report")
				ctx.Become(stateTerminating, "TERMINATING")
				glog.Error("Stop pod failed as init report")
			}
		case EVENT_VM_TIMEOUT:
			reason := "stopping POD timeout"
			ctx.shutdownVM(true, reason)
			ctx.Become(stateTerminating, "TERMINATING")
			glog.Error(reason)
		default:
			glog.Warning("got unexpected event during pod stopping")
		}
	}
}

func stateTerminating(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case EVENT_VM_EXIT:
		glog.Info("Got VM shutdown event while terminating, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onQemuExit(true); !closed {
			ctx.Become(stateDestroying, "DESTROYING")
		}
	case EVENT_VM_KILL:
		glog.Info("Got Qemu force killed message, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onQemuExit(true); !closed {
			ctx.Become(stateDestroying, "DESTROYING")
		}
	case COMMAND_RELEASE:
		glog.Info("vm terminating, got release")
		ctx.reportVmShutdown()
	case COMMAND_ACK:
		ack := ev.(*CommandAck)
		glog.V(1).Infof("[Terminating] Got reply to %d: '%s'", ack.reply, string(ack.msg))
		if ack.reply == INIT_DESTROYPOD {
			glog.Info("POD destroyed ", string(ack.msg))
			ctx.poweroffVM(false, "")
		}
	case ERROR_CMD_FAIL:
		ack := ev.(*CommandError)
		if ack.context.code == INIT_DESTROYPOD {
			glog.Warning("Destroy pod failed")
			ctx.poweroffVM(true, "Destroy pod failed")
		}
	case EVENT_VM_TIMEOUT:
		glog.Warning("Qemu did not exit in time, try to stop it")
		ctx.poweroffVM(true, "vm terminating timeout")
	case ERROR_INTERRUPTED:
		glog.V(1).Info("Connection interrupted while terminating")
	default:
		glog.V(1).Info("got event during terminating")
	}
}

func stateCleaning(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, false); processed {
	} else if processed, success := deviceRemoveHandler(ctx, ev); processed {
		if !success {
			glog.Warning("fail to unplug devices for stop")
			ctx.poweroffVM(true, "fail to unplug devices")
			ctx.Become(stateDestroying, "DESTROYING")
		} else if ctx.deviceReady() {
			//            ctx.reset()
			//            ctx.unsetTimeout()
			//            ctx.reportPodStopped()
			//            glog.V(1).Info("device ready, could run pod.")
			//            ctx.Become(stateInit, "INIT")
			ctx.vm <- &DecodedMessage{
				code:    INIT_READY,
				message: []byte{},
			}
			glog.V(1).Info("device ready, could run pod.")
		}
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.poweroffVM(true, "fail to unplug devices")
		ctx.Become(stateDestroying, "DESTROYING")
	} else {
		switch ev.Event() {
		case COMMAND_RELEASE:
			glog.Info("vm cleaning to idle, got release, quit")
			ctx.reportVmShutdown()
			ctx.Become(stateDestroying, "DESTROYING")
		case EVENT_VM_TIMEOUT:
			glog.Warning("Qemu did not exit in time, try to stop it")
			ctx.poweroffVM(true, "pod stopp/unplug timeout")
			ctx.Become(stateDestroying, "DESTROYING")
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[cleaning] Got reply to %d: '%s'", ack.reply, string(ack.msg))
			if ack.reply == INIT_READY {
				ctx.reset()
				ctx.unsetTimeout()
				ctx.reportPodStopped()
				glog.Info("init has been acknowledged, could run pod.")
				ctx.Become(stateInit, "INIT")
			}
		default:
			glog.V(1).Info("got event message while cleaning")
		}
	}
}

func stateDestroying(ctx *VmContext, ev VmEvent) {
	if processed, _ := deviceRemoveHandler(ctx, ev); processed {
		if closed := ctx.tryClose(); closed {
			glog.Info("resources reclaimed, quit...")
		}
	} else {
		switch ev.Event() {
		case EVENT_VM_EXIT:
			glog.Info("Got VM shutdown event")
			ctx.unsetTimeout()
			if closed := ctx.onQemuExit(false); closed {
				glog.Info("VM Context closed.")
			}
		case EVENT_VM_KILL:
			glog.Info("Got Qemu force killed message")
			ctx.unsetTimeout()
			if closed := ctx.onQemuExit(true); closed {
				glog.Info("VM Context closed.")
			}
		case ERROR_INTERRUPTED:
			glog.V(1).Info("Connection interrupted while destroying")
		case COMMAND_RELEASE:
			glog.Info("vm destroying, got release")
			ctx.reportVmShutdown()
		case EVENT_VM_TIMEOUT:
			glog.Info("Device removing timeout")
			ctx.Close()
		default:
			glog.Warning("got event during vm cleaning up")
		}
	}
}
