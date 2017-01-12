package hypervisor

import (
	"sync"

	"github.com/hyperhq/runv/hypervisor/types"
)

type Fanout struct {
	size     int
	upstream chan *types.VmResponse
	clients  []chan *types.VmResponse

	closeSignal chan *types.VmResponse
	running     bool
	lock        sync.RWMutex
}

// CreateFanout create a new fanout, and if it is non-blocked, it will start
// the fanout goroutine at once, otherwise it will start the goroutine when it
// get the first client
func CreateFanout(upstream chan *types.VmResponse, size int, block bool) *Fanout {
	fo := &Fanout{
		size:        size,
		upstream:    upstream,
		clients:     []chan *types.VmResponse{},
		closeSignal: make(chan *types.VmResponse, 1),
		running:     !block,
		lock:        sync.RWMutex{},
	}

	if !block {
		fo.start()
	}

	return fo
}

func (fo *Fanout) Acquire() (chan *types.VmResponse, error) {
	client := make(chan *types.VmResponse, fo.size)

	fo.lock.Lock()
	first := !fo.running
	fo.running = true
	fo.clients = append(fo.clients, client)
	fo.lock.Unlock()

	if first {
		fo.start()
	}

	return client, nil
}

func (fo *Fanout) Release(client chan *types.VmResponse) error {
	fo.lock.Lock()
	defer fo.lock.Unlock()

	remains := []chan *types.VmResponse{}
	for _, c := range fo.clients {
		if c != client {
			remains = append(remains, c)
			continue
		}
		close(client)
	}
	fo.clients = remains
	return nil
}

func (fo *Fanout) Close() {
	if fo == nil {
		return
	}
	UnblockSend(fo.closeSignal, nil)
	close(fo.closeSignal)
}

func (fo *Fanout) start() {
	go func() {
		next := true
		for next {
			// exit goroutine in case upstream closed or get close signal
			select {
			case rsp, ok := <-fo.upstream:
				if !ok {
					next = false
				} else {
					fo.lock.RLock()
					for _, c := range fo.clients {
						UnblockSend(c, rsp)
					}
					fo.lock.RUnlock()
				}
			case _, _ = <-fo.closeSignal:
				next = false
			}
			// all cleints check and operation should protected
			fo.lock.Lock()
			if !next {
				for _, c := range fo.clients {
					close(c)
				}
				fo.clients = []chan *types.VmResponse{}
				fo.running = false
			}
			fo.lock.Unlock() // don't change this to defer, it is inside loop
		}
	}()
}

func UnblockSend(ch chan *types.VmResponse, u *types.VmResponse) {
	defer func() { recover() }() // in case has closed by someone
	select {
	case ch <- u:
	default:
	}
}
