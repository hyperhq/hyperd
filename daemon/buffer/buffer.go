package buffer

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon/pod"
	apitypes "github.com/hyperhq/hyperd/types"
)

type Buffer struct {
	goroutinesLimit uint64
	goroutinesLock  sync.Mutex
	ch              chan *pod.ContainerBuffer
}

const (
	DefaultBufferChannelSize = 1024
)

func NewBuffer(cfg *apitypes.HyperConfig) *Buffer {
	if cfg.BufferGoroutinesMax == 0 {
		return nil
	}
	if cfg.BufferChannelSize == 0 {
		cfg.BufferChannelSize = DefaultBufferChannelSize
	}

	daemon := &Buffer{
		goroutinesLimit: cfg.BufferGoroutinesMax,
		ch:              make(chan *pod.ContainerBuffer, cfg.BufferChannelSize),
	}

	return daemon
}

func (b *Buffer) CreateContainerInPod(p *pod.XPod, c *apitypes.UserContainer) (string, error) {
	if !p.IsAlive() {
		err := fmt.Errorf("pod is not running")
		p.Log(pod.ERROR, "%v", err)
		return "", err
	}
	if err := p.ReserveContainerName(c); err != nil {
		return "", err
	}

	cb, err := p.AddContainerBuffer(c)
	if err != nil {
		return "", err
	}

	b.goroutinesLock.Lock()
	defer b.goroutinesLock.Unlock()
	if b.goroutinesLimit != 0 {
		b.goroutinesLimit--
		go b.containerHandler(cb)
		glog.V(3).Infof("Put %+v to containerHandler, current limit %v", c, b.goroutinesLimit)
	} else {
		select {
		case b.ch <- cb:
			glog.V(3).Infof("Put %+v to channel", c)
		default:
			err := fmt.Errorf("%+v dropped because channel is full", c)
			glog.Errorf("%s", err)
			p.RemoveContainerBufferAll(cb)
			return "", err
		}
	}

	return cb.Id, nil
}

func (b *Buffer) containerHandler(cb *pod.ContainerBuffer) {
loop:
	for {
		glog.V(3).Infof("Buffer begin to handle %+v", cb)
		id, err := cb.P.DoContainerCreate(cb.Spec, cb.Id)
		if err == nil {
			cb.P.RemoveContainerBuffer(cb)
			glog.V(3).Infof("Buffer handle %+v done, new id is %s", cb, id)
		} else {
			cb.P.RemoveContainerBufferAll(cb)
			glog.Errorf("Buffer handle %+v failed %v", cb, err)
		}

		b.goroutinesLock.Lock()
		select {
		case cb = <-b.ch:
			b.goroutinesLock.Unlock()
		default:
			defer b.goroutinesLock.Unlock()
			b.goroutinesLimit++
			break loop
		}
	}
	glog.V(3).Infof("Channel is empty, current limit %v", b.goroutinesLimit)
}
