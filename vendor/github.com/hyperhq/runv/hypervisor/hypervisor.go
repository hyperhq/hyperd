package hypervisor

import (
	"fmt"
	"time"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (ctx *VmContext) loop() {
	for ctx.handler != nil {
		ev, ok := <-ctx.Hub
		if !ok {
			ctx.Log(ERROR, "hub chan has already been closed")
			break
		} else if ev == nil {
			ctx.Log(DEBUG, "got nil event.")
			continue
		}
		ctx.Log(TRACE, "main event loop got message %d(%s)", ev.Event(), EventString(ev.Event()))
		ctx.handler(ctx, ev)
	}

	// Unless the ctx.Hub channel is drained, processes sending operations can
	// be left hanging waiting for a response. Since the handler is already
	// gone, we return a fail to all these requests.

	ctx.Log(DEBUG, "main event loop exiting")
}

func (ctx *VmContext) watchHyperstart(sendReadyEvent bool) {
	timeout := time.AfterFunc(60*time.Second, func() {
		if ctx.PauseState == PauseStateUnpaused {
			ctx.Log(ERROR, "watch hyperstart timeout")
			ctx.Hub <- &InitFailedEvent{Reason: "watch hyperstart timeout"}
			ctx.hyperstart.Close()
		}
	})
	ctx.Log(DEBUG, "watch hyperstart, send ready: %v", sendReadyEvent)
	for {
		ctx.Log(TRACE, "issue VERSION request for keep-alive test")
		_, err := ctx.hyperstart.APIVersion()
		if err != nil {
			ctx.Log(WARNING, "keep-alive test end with error: %v", err)
			ctx.hyperstart.Close()
			ctx.Hub <- &InitFailedEvent{Reason: "hyperstart failed: " + err.Error()}
			break
		}
		if !timeout.Stop() {
			<-timeout.C
		}
		if sendReadyEvent {
			ctx.Hub <- &InitConnectedEvent{}
			sendReadyEvent = false
		}
		time.Sleep(10 * time.Second)
		timeout.Reset(60 * time.Second)
	}
	timeout.Stop()
}

func (ctx *VmContext) Launch() {
	go ctx.DCtx.Launch(ctx)

	//launch routines
	if ctx.Boot.BootFromTemplate {
		ctx.Log(TRACE, "boot from template")
		ctx.PauseState = PauseStatePaused
		ctx.hyperstart = libhyperstart.NewJsonBasedHyperstart(ctx.Id, ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, false)
		ctx.Hub <- &InitConnectedEvent{}
	} else {
		ctx.hyperstart = libhyperstart.NewJsonBasedHyperstart(ctx.Id, ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, true)
		go ctx.watchHyperstart(true)
	}
	if ctx.LogLevel(DEBUG) {
		go watchVmConsole(ctx)
	}

	go ctx.loop()
}

func VmAssociate(vmId string, hub chan VmEvent, client chan *types.VmResponse, pack []byte) (*VmContext, error) {

	if hlog.IsLogLevel(hlog.DEBUG) {
		hlog.Log(DEBUG, "VM %s trying to reload with serialized data: %s", vmId, string(pack))
	}

	pinfo, err := vmDeserialize(pack)
	if err != nil {
		return nil, err
	}

	if pinfo.Id != vmId {
		return nil, fmt.Errorf("VM ID mismatch, %v vs %v", vmId, pinfo.Id)
	}

	context, err := pinfo.vmContext(hub, client)
	if err != nil {
		return nil, err
	}

	context.hyperstart = libhyperstart.NewJsonBasedHyperstart(context.Id, context.ctlSockAddr(), context.ttySockAddr(), pinfo.HwStat.AttachId, false)
	context.DCtx.Associate(context)

	if context.LogLevel(DEBUG) {
		go watchVmConsole(context)
	}

	context.Become(stateRunning, StateRunning)

	//for _, c := range context.vmSpec.Containers {
	//	context.ptys.ptyConnect(true, c.Process.Terminal, c.Process.Stdio, c.Process.Stderr, nil)
	//	context.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
	//}

	go context.watchHyperstart(false)
	go context.loop()
	return context, nil
}

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	if HDriver.BuildinNetwork() {
		return HDriver.InitNetwork(bIface, bIP, disableIptables)
	}

	return network.InitNetwork(bIface, bIP, disableIptables)
}

func SupportLazyMode() bool {
	return HDriver.SupportLazyMode()
}
