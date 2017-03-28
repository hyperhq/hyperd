package cache

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/hypervisor"
)

type cacheFactory struct {
	b         base.Factory
	cache     chan *hypervisor.Vm
	closed    chan<- int
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func New(cacheSize int, b base.Factory) base.Factory {
	if cacheSize < 1 {
		return b
	}

	cache := make(chan *hypervisor.Vm)
	closed := make(chan int, cacheSize)
	c := cacheFactory{b: b, cache: cache, closed: closed}
	for i := 0; i < cacheSize; i++ {
		c.wg.Add(1)
		go func() {
			for {
				vm, err := b.GetBaseVm()
				if err != nil {
					glog.Errorf("cache factory get error when allocate vm: %v", err)
					c.wg.Done()
					c.CloseFactory()
					return
				}
				glog.V(3).Infof("cache factory get vm from lower layer: %s", vm.Id)

				select {
				case cache <- vm:
					glog.V(3).Infof("cache factory sent one vm: %s", vm.Id)
				case _ = <-closed:
					glog.V(3).Infof("cache factory is going to close")
					vm.Kill()
					c.wg.Done()
					return
				}
			}
		}()
	}
	return &c
}

func (c *cacheFactory) Config() *hypervisor.BootConfig {
	return c.b.Config()
}

func (c *cacheFactory) GetBaseVm() (*hypervisor.Vm, error) {
	vm, ok := <-c.cache
	if ok {
		glog.V(3).Infof("cache factory get vm from cache: %s", vm.Id)
		return vm, nil
	}
	return nil, fmt.Errorf("cache factory is closed")
}

func (c *cacheFactory) CloseFactory() {
	c.closeOnce.Do(func() {
		glog.V(3).Infof("CloseFactory() close cache factory")
		for len(c.closed) < cap(c.closed) { // send sufficient closed signal
			c.closed <- 0
		}
		glog.V(3).Infof("CloseFactory() sent close signal")
		c.wg.Wait()
		close(c.cache)
		c.b.CloseFactory()
	})
}
