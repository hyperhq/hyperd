package hypervisor

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (ctx *VmContext) loop() {
	for ctx.handler != nil {
		ev, ok := <-ctx.Hub
		if !ok {
			glog.Error("hub chan has already been closed")
			break
		} else if ev == nil {
			glog.V(1).Info("got nil event.")
			continue
		}
		glog.V(3).Infof("VM [%s]: main event loop got message %d(%s)", ctx.Id, ev.Event(), EventString(ev.Event()))
		ctx.handler(ctx, ev)
	}

	// Unless the ctx.Hub channel is drained, processes sending operations can
	// be left hanging waiting for a response. Since the handler is already
	// gone, we return a fail to all these requests.

	glog.V(1).Infof("vm %s: main event loop exiting", ctx.Id)
}

func (ctx *VmContext) handlePAEs() {
	ch, err := ctx.hyperstart.ProcessAsyncEvents()
	if err == nil {
		for e := range ch {
			ctx.handleProcessAsyncEvent(&e)
		}
	}
	ctx.hyperstart.Close()
	ctx.Log(ERROR, "hyperstart stopped")
	ctx.Hub <- &Interrupted{Reason: "hyperstart stopped"}
}

func (ctx *VmContext) watchHyperstart(sendReadyEvent bool) {
	timeout := time.AfterFunc(30*time.Second, func() {
		if ctx.PauseState == PauseStateUnpaused {
			ctx.Log(ERROR, "watch hyperstart timeout")
			ctx.Hub <- &InitFailedEvent{Reason: "watch hyperstart timeout"}
			ctx.hyperstart.Close()
		}
	})
	for {
		_, err := ctx.hyperstart.APIVersion()
		if err != nil {
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
		timeout.Reset(30 * time.Second)
	}
	timeout.Stop()
}

func (ctx *VmContext) Launch() {
	go ctx.DCtx.Launch(ctx)

	//launch routines
	if ctx.Boot.BootFromTemplate {
		glog.V(3).Info("boot from template")
		ctx.PauseState = PauseStatePaused
		ctx.hyperstart = libhyperstart.NewJsonBasedHyperstart(ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, false)
		ctx.Hub <- &InitConnectedEvent{}
	} else {
		ctx.hyperstart = libhyperstart.NewJsonBasedHyperstart(ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, true)
		go ctx.watchHyperstart(true)
	}
	if glog.V(1) {
		go watchVmConsole(ctx)
	}

	go ctx.loop()
	go ctx.handlePAEs()
}

func VmAssociate(vmId string, hub chan VmEvent, client chan *types.VmResponse, pack []byte) (*VmContext, error) {

	if glog.V(1) {
		glog.Infof("VM %s trying to reload with serialized data: %s", vmId, string(pack))
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

	context.hyperstart = libhyperstart.NewJsonBasedHyperstart(context.ctlSockAddr(), context.ttySockAddr(), pinfo.HwStat.AttachId, false)
	context.DCtx.Associate(context)

	if glog.V(1) {
		go watchVmConsole(context)
	}

	context.Become(stateRunning, StateRunning)

	//for _, c := range context.vmSpec.Containers {
	//	context.ptys.ptyConnect(true, c.Process.Terminal, c.Process.Stdio, c.Process.Stderr, nil)
	//	context.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
	//}

	go context.watchHyperstart(false)
	go context.handlePAEs()
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
