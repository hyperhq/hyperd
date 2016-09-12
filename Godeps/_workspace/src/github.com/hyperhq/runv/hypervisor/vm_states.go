package hypervisor

import (
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/pod"
)

// states
const (
	StateInit        = "INIT"
	StatePreparing   = "PREPARING"
	StateStarting    = "STARTING"
	StateRunning     = "RUNNING"
	StatePodStopping = "STOPPING"
	StateCleaning    = "CLEANING"
	StateTerminating = "TERMINATING"
	StateDestroying  = "DESTROYING"
	StateNone        = "NONE"
)

func (ctx *VmContext) timedKill(seconds int) {
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if ctx != nil && ctx.handler != nil {
			ctx.DCtx.Kill(ctx)
		}
	})
}

func (ctx *VmContext) onVmExit(reclaim bool) bool {
	glog.V(1).Info("VM has exit...")
	ctx.reportVmShutdown()
	ctx.setTimeout(60)

	if reclaim {
		ctx.reclaimDevice()
	}

	return ctx.tryClose()
}

func (ctx *VmContext) reclaimDevice() {
	ctx.releaseNetwork()
}

func (ctx *VmContext) detachDevice() {
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

	pendings := ctx.ptys.pendingTtys
	ctx.ptys.pendingTtys = []*AttachCommand{}
	for _, acmd := range pendings {
		idx := ctx.Lookup(acmd.Container)
		if idx >= 0 {
			glog.Infof("attach pending client for %s", acmd.Container)
			ctx.attachTty2Container(&ctx.vmSpec.Containers[idx].Process, acmd)
		} else {
			glog.Infof("not attach for %s", acmd.Container)
			ctx.ptys.pendingTtys = append(ctx.ptys.pendingTtys, acmd)
		}
	}

	ctx.allocateDevices()

	return true
}

func (ctx *VmContext) prepareContainer(cmd *NewContainerCommand) *hyperstartapi.Container {
	ctx.lock.Lock()

	idx := len(ctx.vmSpec.Containers)
	vmContainer := &hyperstartapi.Container{}

	ctx.initContainerInfo(idx, vmContainer, cmd.container)
	ctx.setContainerInfo(idx, vmContainer, cmd.info)

	vmContainer.Sysctl = cmd.container.Sysctl
	vmContainer.Process.Stdio = ctx.ptys.attachId
	ctx.ptys.attachId++
	if !cmd.container.Tty {
		vmContainer.Process.Stderr = ctx.ptys.attachId
		ctx.ptys.attachId++
	}

	ctx.userSpec.Containers = append(ctx.userSpec.Containers, *cmd.container)
	ctx.vmSpec.Containers = append(ctx.vmSpec.Containers, *vmContainer)

	ctx.lock.Unlock()

	pendings := ctx.ptys.pendingTtys
	ctx.ptys.pendingTtys = []*AttachCommand{}
	for _, acmd := range pendings {
		if idx == ctx.Lookup(acmd.Container) {
			glog.Infof("attach pending client for %s", acmd.Container)
			ctx.attachTty2Container(&ctx.vmSpec.Containers[idx].Process, acmd)
		} else {
			glog.Infof("not attach for %s", acmd.Container)
			ctx.ptys.pendingTtys = append(ctx.ptys.pendingTtys, acmd)
		}
	}

	return vmContainer
}

func (ctx *VmContext) newContainer(cmd *NewContainerCommand) {
	c := ctx.prepareContainer(cmd)
	glog.Infof("start sending INIT_NEWCONTAINER")
	ctx.vm <- &hyperstartCmd{
		Code:    hyperstartapi.INIT_NEWCONTAINER,
		Message: c,
	}
	glog.Infof("sent INIT_NEWCONTAINER")
}

func (ctx *VmContext) updateInterface(index int, result chan<- error) {
	if _, ok := ctx.devices.networkMap[index]; !ok {
		result <- fmt.Errorf("can't find interface whose index is %d", index)
		return
	}

	inf := hyperstartapi.NetworkInf{
		Device:    ctx.devices.networkMap[index].DeviceName,
		IpAddress: ctx.devices.networkMap[index].IpAddr,
		NetMask:   ctx.devices.networkMap[index].NetMask,
	}

	ctx.vm <- &hyperstartCmd{
		Code:    hyperstartapi.INIT_SETUPINTERFACE,
		Message: inf,
		result:  result,
	}
}

func (ctx *VmContext) setWindowSize(containerId, execId string, size *WindowSize) {
	var session uint64
	if execId != "" {
		exec, ok := ctx.vmExec[execId]
		if !ok {
			glog.Errorf("cannot find exec %s", execId)
			return
		}

		session = exec.Process.Stdio
	} else if containerId != "" {
		idx := ctx.Lookup(containerId)
		if idx < 0 || idx > len(ctx.vmSpec.Containers) {
			glog.Errorf("cannot find container %s", containerId)
			return
		}

		session = ctx.vmSpec.Containers[idx].Process.Stdio
	} else {
		glog.Error("no container or exec is specified")
		return
	}

	if !ctx.ptys.isTty(session) {
		glog.Error("the session is not a tty, doesn't support resize.")
		return
	}

	cmd := hyperstartapi.WindowSizeMessage{
		Seq:    session,
		Row:    size.Row,
		Column: size.Column,
	}

	ctx.vm <- &hyperstartCmd{
		Code:    hyperstartapi.INIT_WINSIZE,
		Message: cmd,
	}
}

func (ctx *VmContext) onlineCpuMem(cmd *OnlineCpuMemCommand) {
	ctx.vm <- &hyperstartCmd{
		Code: hyperstartapi.INIT_ONLINECPUMEM,
	}
}

func (ctx *VmContext) execCmd(execId string, cmd *hyperstartapi.ExecCommand, tty *TtyIO, result chan<- error) {
	cmd.Process.Stdio = ctx.ptys.nextAttachId()
	if !cmd.Process.Terminal {
		cmd.Process.Stderr = ctx.ptys.nextAttachId()
	}
	ctx.vmExec[execId] = cmd
	ctx.ptys.ptyConnect(false, cmd.Process.Terminal, cmd.Process.Stdio, cmd.Process.Stderr, tty)
	ctx.vm <- &hyperstartCmd{
		Code:    hyperstartapi.INIT_EXECCMD,
		Message: cmd,
		result:  result,
	}
}

func (ctx *VmContext) killCmd(container string, signal syscall.Signal, result chan<- error) {
	ctx.vm <- &hyperstartCmd{
		Code: hyperstartapi.INIT_KILLCONTAINER,
		Message: hyperstartapi.KillCommand{
			Container: container,
			Signal:    signal,
		},
		result: result,
	}
}

func (ctx *VmContext) attachCmd(cmd *AttachCommand, result chan<- error) {
	idx := ctx.Lookup(cmd.Container)
	if cmd.Container != "" && idx < 0 {
		ctx.ptys.pendingTtys = append(ctx.ptys.pendingTtys, cmd)
		glog.V(1).Infof("attachment %s is pending", cmd.Container)
		result <- nil
		return
	} else if idx < 0 || idx > len(ctx.vmSpec.Containers) || ctx.vmSpec.Containers[idx].Process.Stdio == 0 {
		result <- fmt.Errorf("tty is not configured for %s", cmd.Container)
		return
	}
	ctx.attachTty2Container(&ctx.vmSpec.Containers[idx].Process, cmd)
	if cmd.Size != nil {
		ctx.setWindowSize(cmd.Container, "", cmd.Size)
	}

	result <- nil
}

func (ctx *VmContext) attachTty2Container(process *hyperstartapi.Process, cmd *AttachCommand) {
	session := process.Stdio
	ctx.ptys.ptyConnect(true, process.Terminal, session, process.Stderr, cmd.Streams)
	glog.V(1).Infof("Connecting tty for %s on session %d", cmd.Container, session)
}

func (ctx *VmContext) startPod() {
	ctx.vm <- &hyperstartCmd{
		Code:    hyperstartapi.INIT_STARTPOD,
		Message: ctx.vmSpec,
	}
}

func (ctx *VmContext) stopPod() {
	ctx.setTimeout(30)
	ctx.vm <- &hyperstartCmd{
		Code: hyperstartapi.INIT_STOPPOD,
	}
}

func (ctx *VmContext) exitVM(err bool, msg string, hasPod bool, wait bool) {
	ctx.wait = wait
	if hasPod {
		ctx.shutdownVM(err, msg)
		ctx.Become(stateTerminating, StateTerminating)
	} else {
		ctx.poweroffVM(err, msg)
		ctx.Become(stateDestroying, StateDestroying)
	}
}

func (ctx *VmContext) shutdownVM(err bool, msg string) {
	if err {
		ctx.reportVmFault(msg)
		glog.Error("Shutting down because of an exception: ", msg)
	}
	ctx.setTimeout(10)
	ctx.vm <- &hyperstartCmd{Code: hyperstartapi.INIT_DESTROYPOD}
}

func (ctx *VmContext) poweroffVM(err bool, msg string) {
	if err {
		ctx.reportVmFault(msg)
		glog.Error("Shutting down because of an exception: ", msg)
	}
	ctx.DCtx.Shutdown(ctx)
	ctx.timedKill(10)
}

func (ctx *VmContext) handleGenericOperation(goe *GenericOperation) {
	for _, allowd := range goe.State {
		if ctx.current == allowd {
			glog.V(3).Infof("handle GenericOperation(%s) on state(%s)", goe.OpName, ctx.current)
			goe.OpFunc(ctx, goe.Result)
			return
		}
	}

	glog.V(3).Infof("GenericOperation(%s) is unsupported on state(%s)", goe.OpName, ctx.current)
	goe.Result <- fmt.Errorf("GenericOperation(%s) is unsupported on state(%s)", goe.OpName, ctx.current)
}

// state machine
func commonStateHandler(ctx *VmContext, ev VmEvent, hasPod bool) bool {
	processed := true
	switch ev.Event() {
	case EVENT_VM_EXIT:
		glog.Info("Got VM shutdown event, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onVmExit(hasPod); !closed {
			ctx.Become(stateDestroying, StateDestroying)
		}
	case ERROR_INTERRUPTED:
		glog.Info("Connection interrupted, quit...")
		ctx.exitVM(true, "connection to VM broken", false, false)
		ctx.onVmExit(hasPod)
	case COMMAND_SHUTDOWN:
		glog.Info("got shutdown command, shutting down")
		ctx.exitVM(false, "", hasPod, ev.(*ShutdownCommand).Wait)
	case GENERIC_OPERATION:
		ctx.handleGenericOperation(ev.(*GenericOperation))
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
	case EVENT_DEV_SKIP:
	case EVENT_INTERFACE_ADD:
		info := ev.(*InterfaceCreated)
		ctx.interfaceCreated(info, false, ctx.Hub)
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
		go ctx.ReleaseInterface(n.Index, nic.IpAddr, nic.Fd, maps)
	default:
		processed = false
	}
	return processed, success
}

func unexpectedEventHandler(ctx *VmContext, ev VmEvent, state string) {
	switch ev.Event() {
	case COMMAND_RUN_POD,
		COMMAND_STOP_POD,
		COMMAND_REPLACE_POD,
		COMMAND_SHUTDOWN,
		COMMAND_RELEASE,
		COMMAND_PAUSEVM:
		ctx.reportUnexpectedRequest(ev, state)
	default:
		glog.Warning("got unexpected event during ", state)
	}
}

func initFailureHandler(ctx *VmContext, ev VmEvent) bool {
	processed := true
	switch ev.Event() {
	case ERROR_INIT_FAIL: // VM connection Failure
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
		ctx.Become(stateDestroying, StateDestroying)
	} else {
		switch ev.Event() {
		case EVENT_VM_START_FAILED:
			msg := ev.(*VmStartFailEvent)
			glog.Errorf("VM start failed: %s, go to cleaning up", msg.Message)
			ctx.reportVmFault("VM did not start up properly, go to cleaning up")
			ctx.Close()
		case EVENT_INIT_CONNECTED:
			glog.Info("begin to wait vm commands")
			ctx.reportVmRun()
		case COMMAND_RELEASE:
			glog.Info("no pod on vm, got release, quit.")
			ctx.shutdownVM(false, "")
			ctx.Become(stateDestroying, StateDestroying)
			ctx.reportVmShutdown()
		case COMMAND_NEWCONTAINER:
			ctx.newContainer(ev.(*NewContainerCommand))
		case COMMAND_ONLINECPUMEM:
			ctx.onlineCpuMem(ev.(*OnlineCpuMemCommand))
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			ctx.setWindowSize(cmd.ContainerId, cmd.ExecId, cmd.Size)
		case COMMAND_RUN_POD, COMMAND_REPLACE_POD:
			glog.Info("got spec, prepare devices")
			if ok := ctx.prepareDevice(ev.(*RunPodCommand)); ok {
				ctx.setTimeout(60)
				ctx.Become(stateStarting, StateStarting)
			}
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[init] got init ack to %d", ack.reply)
			if ack.reply.Code == hyperstartapi.INIT_NEWCONTAINER {
				glog.Infof("Get ack for new container")

				// start stdin. TODO: find the correct idx if parallel multi INIT_NEWCONTAINER
				idx := len(ctx.vmSpec.Containers) - 1
				c := ctx.vmSpec.Containers[idx]
				ctx.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
			}
		default:
			unexpectedEventHandler(ctx, ev, "pod initiating")
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
		ctx.Become(stateTerminating, StateTerminating)
	} else {
		switch ev.Event() {
		case EVENT_VM_START_FAILED:
			glog.Info("VM did not start up properly, go to cleaning up")
			if closed := ctx.onVmExit(true); !closed {
				ctx.Become(stateDestroying, StateDestroying)
			}
		case EVENT_INIT_CONNECTED:
			glog.Info("begin to wait vm commands")
			ctx.reportVmRun()
		case COMMAND_RELEASE:
			glog.Info("pod starting, got release, please wait")
			ctx.reportBusy("")
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			ctx.setWindowSize(cmd.ContainerId, cmd.ExecId, cmd.Size)
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[starting] got init ack to %d", ack.reply)
			if ack.reply.Code == hyperstartapi.INIT_STARTPOD {
				ctx.unsetTimeout()
				var pinfo []byte = []byte{}
				persist, err := ctx.dump()
				if err == nil {
					buf, err := persist.serialize()
					if err == nil {
						pinfo = buf
					}
				}
				for _, c := range ctx.vmSpec.Containers {
					ctx.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
				}
				ctx.reportSuccess("Start POD success", pinfo)
				ctx.Become(stateRunning, StateRunning)
				glog.Info("pod start success ", string(ack.msg))
			}
		case ERROR_CMD_FAIL:
			ack := ev.(*CommandError)
			if ack.reply.Code == hyperstartapi.INIT_STARTPOD {
				reason := "Start POD failed"
				ctx.shutdownVM(true, reason)
				ctx.Become(stateTerminating, StateTerminating)
				glog.Error(reason)
			}
		case EVENT_VM_TIMEOUT:
			reason := "Start POD timeout"
			ctx.shutdownVM(true, reason)
			ctx.Become(stateTerminating, StateTerminating)
			glog.Error(reason)
		default:
			unexpectedEventHandler(ctx, ev, "pod initiating")
		}
	}
}

func stateRunning(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, true); processed {
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.shutdownVM(true, "Fail during reconnect to a running pod")
		ctx.Become(stateTerminating, StateTerminating)
	} else {
		switch ev.Event() {
		case COMMAND_STOP_POD:
			ctx.stopPod()
			ctx.Become(statePodStopping, StatePodStopping)
		case COMMAND_RELEASE:
			glog.Info("pod is running, got release command, let VM fly")
			ctx.Become(nil, StateNone)
			ctx.reportSuccess("", nil)
		case COMMAND_NEWCONTAINER:
			ctx.newContainer(ev.(*NewContainerCommand))
		case COMMAND_WINDOWSIZE:
			cmd := ev.(*WindowSizeCommand)
			ctx.setWindowSize(cmd.ContainerId, cmd.ExecId, cmd.Size)
		case EVENT_POD_FINISH:
			result := ev.(*PodFinished)
			ctx.reportPodFinished(result)
			ctx.exitVM(false, "", true, false)
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[running] got init ack to %d", ack.reply)

			if ack.reply.Code == hyperstartapi.INIT_NEWCONTAINER {
				glog.Infof("Get ack for new container")
				// start stdin. TODO: find the correct idx if parallel multi INIT_NEWCONTAINER
				idx := len(ctx.vmSpec.Containers) - 1
				c := ctx.vmSpec.Containers[idx]
				ctx.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
			}
		case COMMAND_GET_POD_STATS:
			ctx.reportPodStats(ev)
		case EVENT_INTERFACE_EJECTED:
			ctx.releaseNetworkByLinkIndex((ev.(*NetDevRemovedEvent)).Index)
			glog.V(1).Info("releaseNetworkByLinkIndex:", (ev.(*NetDevRemovedEvent)).Index)
		default:
			unexpectedEventHandler(ctx, ev, "pod running")
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
			ctx.Become(stateTerminating, StateTerminating)
			ctx.reportVmShutdown()
		case EVENT_POD_FINISH:
			glog.Info("POD stopped")
			ctx.detachDevice()
			ctx.Become(stateCleaning, StateCleaning)
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[Stopping] got init ack to %d", ack.reply.Code)
			if ack.reply.Code == hyperstartapi.INIT_STOPPOD {
				glog.Info("POD stopped ", string(ack.msg))
				ctx.detachDevice()
				ctx.Become(stateCleaning, StateCleaning)
			}
		case ERROR_CMD_FAIL:
			ack := ev.(*CommandError)
			if ack.reply.Code == hyperstartapi.INIT_STOPPOD {
				ctx.unsetTimeout()
				ctx.shutdownVM(true, "Stop pod failed as init report")
				ctx.Become(stateTerminating, StateTerminating)
				glog.Error("Stop pod failed as init report")
			}
		case EVENT_VM_TIMEOUT:
			reason := "stopping POD timeout"
			ctx.shutdownVM(true, reason)
			ctx.Become(stateTerminating, StateTerminating)
			glog.Error(reason)
		default:
			unexpectedEventHandler(ctx, ev, "pod stopping")
		}
	}
}

func stateTerminating(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case EVENT_VM_EXIT:
		glog.Info("Got VM shutdown event while terminating, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onVmExit(true); !closed {
			ctx.Become(stateDestroying, StateDestroying)
		}
	case EVENT_VM_KILL:
		glog.Info("Got VM force killed message, go to cleaning up")
		ctx.unsetTimeout()
		if closed := ctx.onVmExit(true); !closed {
			ctx.Become(stateDestroying, StateDestroying)
		}
	case COMMAND_RELEASE:
		glog.Info("vm terminating, got release")
		ctx.reportVmShutdown()
	case COMMAND_ACK:
		ack := ev.(*CommandAck)
		glog.V(1).Infof("[Terminating] Got reply to %d: '%s'", ack.reply, string(ack.msg))
		if ack.reply.Code == hyperstartapi.INIT_DESTROYPOD {
			glog.Info("POD destroyed ", string(ack.msg))
			ctx.poweroffVM(false, "")
		}
	case ERROR_CMD_FAIL:
		ack := ev.(*CommandError)
		if ack.reply.Code == hyperstartapi.INIT_DESTROYPOD {
			glog.Warning("Destroy pod failed")
			ctx.poweroffVM(true, "Destroy pod failed")
		}
	case EVENT_VM_TIMEOUT:
		glog.Warning("VM did not exit in time, try to stop it")
		ctx.poweroffVM(true, "vm terminating timeout")
	case ERROR_INTERRUPTED:
		glog.V(1).Info("Connection interrupted while terminating")
	case GENERIC_OPERATION:
		ctx.handleGenericOperation(ev.(*GenericOperation))
	default:
		unexpectedEventHandler(ctx, ev, "terminating")
	}
}

func stateCleaning(ctx *VmContext, ev VmEvent) {
	if processed := commonStateHandler(ctx, ev, false); processed {
	} else if processed, success := deviceRemoveHandler(ctx, ev); processed {
		if !success {
			glog.Warning("fail to unplug devices for stop")
			ctx.poweroffVM(true, "fail to unplug devices")
			ctx.Become(stateDestroying, StateDestroying)
		} else if ctx.deviceReady() {
			//            ctx.reset()
			//            ctx.unsetTimeout()
			//            ctx.reportPodStopped()
			//            glog.V(1).Info("device ready, could run pod.")
			//            ctx.Become(stateInit, StateInit)
			ctx.vm <- &hyperstartCmd{
				Code: hyperstartapi.INIT_READY,
			}
			glog.V(1).Info("device ready, could run pod.")
		}
	} else if processed := initFailureHandler(ctx, ev); processed {
		ctx.poweroffVM(true, "fail to unplug devices")
		ctx.Become(stateDestroying, StateDestroying)
	} else {
		switch ev.Event() {
		case COMMAND_RELEASE:
			glog.Info("vm cleaning to idle, got release, quit")
			ctx.reportVmShutdown()
			ctx.Become(stateDestroying, StateDestroying)
		case EVENT_VM_TIMEOUT:
			glog.Warning("VM did not exit in time, try to stop it")
			ctx.poweroffVM(true, "pod stopp/unplug timeout")
			ctx.Become(stateDestroying, StateDestroying)
		case COMMAND_ACK:
			ack := ev.(*CommandAck)
			glog.V(1).Infof("[cleaning] Got reply to %d: '%s'", ack.reply.Code, string(ack.msg))
			if ack.reply.Code == hyperstartapi.INIT_READY {
				ctx.reset()
				ctx.unsetTimeout()
				ctx.reportPodStopped()
				glog.Info("init has been acknowledged, could run pod.")
				ctx.Become(stateInit, StateInit)
			}
		default:
			unexpectedEventHandler(ctx, ev, "cleaning")
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
			if closed := ctx.onVmExit(false); closed {
				glog.Info("VM Context closed.")
			}
		case EVENT_VM_KILL:
			glog.Info("Got VM force killed message")
			ctx.unsetTimeout()
			if closed := ctx.onVmExit(true); closed {
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
		case GENERIC_OPERATION:
			ctx.handleGenericOperation(ev.(*GenericOperation))
		default:
			unexpectedEventHandler(ctx, ev, "vm cleaning up")
		}
	}
}
