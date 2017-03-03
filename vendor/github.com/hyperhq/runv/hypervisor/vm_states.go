package hypervisor

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/glog"
)

// states
const (
	StateRunning     = "RUNNING"
	StateTerminating = "TERMINATING"
	StateNone        = "NONE"
)

func (ctx *VmContext) timedKill(seconds int) {
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if ctx != nil && ctx.handler != nil {
			ctx.DCtx.Kill(ctx)
		}
	})
}

func (ctx *VmContext) newContainer(id string) error {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	c, ok := ctx.containers[id]
	if ok {
		glog.Infof("start sending INIT_NEWCONTAINER")
		var err error
		c.stdinPipe, c.stdoutPipe, c.stderrPipe, err = ctx.hyperstart.NewContainer(c.VmSpec())
		if err == nil && c.tty != nil {
			go streamCopy(c.tty, c.stdinPipe, c.stdoutPipe, c.stderrPipe)
		}
		glog.Infof("sent INIT_NEWCONTAINER")
		return err
	} else {
		return fmt.Errorf("container %s not exist", id)
	}
}

func (ctx *VmContext) updateInterface(id string) error {
	if inf := ctx.networks.getInterface(id); inf == nil {
		return fmt.Errorf("can't find interface whose ID is %s", id)
	} else {
		return ctx.hyperstart.UpdateInterface(inf.DeviceName, inf.IpAddr, inf.NetMask)
	}
}

// TODO remove attachCmd and move streamCopy to hyperd
func (ctx *VmContext) attachCmd(cmd *AttachCommand) error {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	c, ok := ctx.containers[cmd.Container]
	if !ok {
		estr := fmt.Sprintf("cannot find container %s to attach", cmd.Container)
		ctx.Log(ERROR, estr)
		return errors.New(estr)
	}

	if c.tty != nil {
		return fmt.Errorf("we can attach only once")
	}
	c.tty = cmd.Streams
	if c.stdinPipe != nil {
		go streamCopy(c.tty, c.stdinPipe, c.stdoutPipe, c.stderrPipe)
	}

	return nil
}

// TODO move this logic to hyperd
type TtyIO struct {
	Stdin  io.ReadCloser
	Stdout io.Writer
	Stderr io.Writer
}

func (tty *TtyIO) Close() {
	glog.V(1).Info("Close tty ")

	if tty.Stdin != nil {
		tty.Stdin.Close()
	}
	cf := func(w io.Writer) {
		if w == nil {
			return
		}
		if c, ok := w.(io.WriteCloser); ok {
			c.Close()
		}
	}
	cf(tty.Stdout)
	cf(tty.Stderr)
}

// TODO move this logic to hyperd
func streamCopy(tty *TtyIO, stdinPipe io.WriteCloser, stdoutPipe, stderrPipe io.ReadCloser) {
	var wg sync.WaitGroup
	// old way cleanup all(expect stdinPipe) no matter what kinds of fails, TODO: change it
	var once sync.Once
	cleanup := func() {
		tty.Close()
		// stdinPipe is directly closed in the first go routine
		stdoutPipe.Close()
		if stderrPipe != nil {
			stderrPipe.Close()
		}
	}
	if tty.Stdin != nil {
		wg.Add(1)
		go func() {
			_, err := io.Copy(stdinPipe, tty.Stdin)
			stdinPipe.Close()
			if err != nil {
				// we should not call cleanup when tty.Stdin reaches EOF
				once.Do(cleanup)
			}
			wg.Done()
		}()
	}
	if tty.Stdout != nil {
		wg.Add(1)
		go func() {
			io.Copy(tty.Stdout, stdoutPipe)
			once.Do(cleanup)
			wg.Done()
		}()
	}
	if tty.Stderr != nil && stderrPipe != nil {
		wg.Add(1)
		go func() {
			io.Copy(tty.Stderr, stderrPipe)
			once.Do(cleanup)
			wg.Done()
		}()
	}
	wg.Wait()
	once.Do(cleanup)
}

func (ctx *VmContext) startPod() {
	err := ctx.hyperstart.StartSandbox(ctx.networks.sandboxInfo())
	if err == nil {
		ctx.Log(INFO, "pod start success")
		ctx.reportSuccess("Start POD success", []byte{})
	} else {
		reason := "Start POD failed"
		ctx.reportVmFault(reason)
		ctx.Log(ERROR, reason)
	}
}

func (ctx *VmContext) shutdownVM() {
	ctx.setTimeout(10)
	err := ctx.hyperstart.DestroySandbox()
	if err == nil {
		glog.Info("POD destroyed")
		ctx.poweroffVM(false, "")
	} else {
		glog.Warning("Destroy pod failed")
		ctx.poweroffVM(true, "Destroy pod failed")
		ctx.Close()
	}
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

// state machine
func unexpectedEventHandler(ctx *VmContext, ev VmEvent, state string) {
	switch ev.Event() {
	case COMMAND_SHUTDOWN,
		COMMAND_RELEASE,
		COMMAND_PAUSEVM:
		ctx.reportUnexpectedRequest(ev, state)
	default:
		glog.Warning("got unexpected event during ", state)
	}
}

func stateRunning(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case COMMAND_SHUTDOWN:
		ctx.Log(INFO, "got shutdown command, shutting down")
		go ctx.shutdownVM()
		ctx.Become(stateTerminating, StateTerminating)
	case COMMAND_RELEASE:
		ctx.Log(INFO, "pod is running, got release command, let VM fly")
		ctx.Become(nil, StateNone)
		ctx.reportSuccess("", nil)
	case EVENT_INIT_CONNECTED:
		ctx.Log(INFO, "hyperstart is ready to accept vm commands")
		ctx.reportVmRun()
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
	case EVENT_VM_TIMEOUT:
		glog.Warning("VM did not exit in time, try to stop it")
		ctx.poweroffVM(true, "vm terminating timeout")
		ctx.Close()
	case ERROR_INTERRUPTED:
		interruptEv := ev.(*Interrupted)
		glog.V(1).Info("Connection interrupted while terminating: %s", interruptEv.Reason)
	default:
		unexpectedEventHandler(ctx, ev, "terminating")
	}
}
