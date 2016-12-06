package hypervisor

import (
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
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

func (ctx *VmContext) newContainer(id string, result chan<- error) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	c, ok := ctx.containers[id]
	if ok {
		glog.Infof("start sending INIT_NEWCONTAINER")
		ctx.vm <- &hyperstartCmd{
			Code:    hyperstartapi.INIT_NEWCONTAINER,
			Message: c.VmSpec(),
			result:  result,
		}
		glog.Infof("sent INIT_NEWCONTAINER")
	} else {
		result <- fmt.Errorf("container %s not exist", id)
	}
}

func (ctx *VmContext) updateInterface(id string, result chan<- error) {
	if inf := ctx.networks.getInterface(id); inf == nil {
		result <- fmt.Errorf("can't find interface whose ID is %s", id)
		return
	} else {
		ctx.vm <- &hyperstartCmd{
			Code: hyperstartapi.INIT_SETUPINTERFACE,
			Message: hyperstartapi.NetworkInf{
				Device:    inf.DeviceName,
				IpAddress: inf.IpAddr,
				NetMask:   inf.NetMask,
			},
			result: result,
		}
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
		ctx.lock.Lock()
		defer ctx.lock.Unlock()

		c, ok := ctx.containers[containerId]
		if !ok {
			glog.Errorf("cannot find container %s", containerId)
			return
		}

		session = c.process.Stdio
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
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	c, ok := ctx.containers[cmd.Container]
	if !ok {
		estr := fmt.Sprintf("cannot find container %s to attach", cmd.Container)
		ctx.Log(ERROR, estr)
		result <- errors.New(estr)
		return
	}

	ctx.attachTty2Container(c.process, cmd)
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
		Message: ctx.networks.sandboxInfo(),
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
		glog.Error("Shutting down because of an exception: ", msg)
	}
	//REFACTOR: kill directly instead of DCtx.Shutdown() and send shutdown information
	ctx.Log(INFO, "poweroff vm based on command: %s", msg)
	if ctx != nil && ctx.handler != nil {
		ctx.DCtx.Kill(ctx)
	}
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
func unexpectedEventHandler(ctx *VmContext, ev VmEvent, state string) {
	switch ev.Event() {
	case COMMAND_STOP_POD,
		COMMAND_SHUTDOWN,
		COMMAND_RELEASE,
		COMMAND_PAUSEVM:
		ctx.reportUnexpectedRequest(ev, state)
	default:
		glog.Warning("got unexpected event during ", state)
	}
}

func stateRunning(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case COMMAND_ONLINECPUMEM:
		ctx.onlineCpuMem(ev.(*OnlineCpuMemCommand))
	case COMMAND_WINDOWSIZE:
		cmd := ev.(*WindowSizeCommand)
		ctx.setWindowSize(cmd.ContainerId, cmd.ExecId, cmd.Size)
	case COMMAND_GET_POD_STATS:
		ctx.reportPodStats(ev)
	case COMMAND_SHUTDOWN:
		ctx.Log(INFO, "got shutdown command, shutting down")
		ctx.shutdownVM(false, "")
		ctx.Become(stateTerminating, StateTerminating)
	case COMMAND_RELEASE:
		ctx.Log(INFO, "pod is running, got release command, let VM fly")
		ctx.Become(nil, StateNone)
		ctx.reportSuccess("", nil)
	case COMMAND_STOP_POD: // REFACTOR: deprecated, will ignore this command
		ctx.Log(INFO, "REFACTOR: ignore COMMAND_STOP_POD")
	case COMMAND_ACK:
		ack := ev.(*CommandAck)
		ctx.Log(DEBUG, "[running] got hyperstart ack to %d", ack.reply.Code)
		switch ack.reply.Code {
		case hyperstartapi.INIT_STARTPOD:
			ctx.Log(INFO, "pod start success ", string(ack.msg))
			ctx.reportSuccess("Start POD success", []byte{})
			//TODO: the payload is the persist info, will deal with this later
		default:
		}
	case EVENT_INIT_CONNECTED:
		ctx.Log(INFO, "hyperstart is ready to accept vm commands")
		ctx.reportVmRun()
	case EVENT_POD_FINISH: // REFACTOR: can i ignore it? OK, will ignore it
		ctx.Log(INFO, "REFACTOR: ignore EVENT_POD_FINISH")
	case EVENT_VM_EXIT, ERROR_VM_START_FAILED:
		ctx.Log(INFO, "VM has exit, or not started at all (%d)", ev.Event())
		ctx.reportVmShutdown()
		ctx.Close()
	case EVENT_VM_TIMEOUT: // REFACTOR: we do not set timeout for prepare devices after the refactor, then we do not need wait this event any more
		ctx.Log(ERROR, "REFACTOR: should be no time in running state at all")
	case ERROR_INIT_FAIL: // VM connection Failure
		reason := ev.(*InitFailedEvent).Reason
		ctx.Log(ERROR, reason)
		ctx.poweroffVM(true, "connection to vm broken")
		ctx.Close()
	case ERROR_INTERRUPTED:
		glog.Info("Connection interrupted, quit...")
		ctx.poweroffVM(true, "connection to vm broken")
		ctx.Close()
	case ERROR_CMD_FAIL:
		ack := ev.(*CommandError)
		switch ack.reply.Code {
		case hyperstartapi.INIT_NEWCONTAINER:
			//TODO: report fail message to the caller
		case hyperstartapi.INIT_STARTPOD:
			reason := "Start POD failed"
			ctx.reportVmFault(reason)
			ctx.Log(ERROR, reason)
		default:
		}
	case GENERIC_OPERATION:
		ctx.handleGenericOperation(ev.(*GenericOperation))
	default:
		unexpectedEventHandler(ctx, ev, "pod running")
	}
}

func stateTerminating(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case EVENT_VM_EXIT:
		glog.Info("Got VM shutdown event while terminating, go to cleaning up")
		ctx.reportVmShutdown()
		ctx.Close()
	case EVENT_VM_KILL:
		glog.Info("Got VM force killed message, go to cleaning up")
		ctx.reportVmShutdown()
		ctx.Close()
	case COMMAND_RELEASE:
		glog.Info("vm terminating, got release")
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
			ctx.Close()
		}
	case EVENT_VM_TIMEOUT:
		glog.Warning("VM did not exit in time, try to stop it")
		ctx.poweroffVM(true, "vm terminating timeout")
		ctx.Close()
	case ERROR_INTERRUPTED:
		interruptEv := ev.(*Interrupted)
		glog.V(1).Info("Connection interrupted while terminating: %s", interruptEv.Reason)
	case GENERIC_OPERATION:
		ctx.handleGenericOperation(ev.(*GenericOperation))
	default:
		unexpectedEventHandler(ctx, ev, "terminating")
	}
}
