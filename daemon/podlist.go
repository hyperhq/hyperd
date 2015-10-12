package daemon

import (
	"sync"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

type PodList struct {
	pods map[string]*Pod
	sync.RWMutex
}

func NewPodList() *PodList {
	return &PodList{
		pods: make(map[string]*Pod),
	}
}

func (pl *PodList) Get(id string) (*Pod, bool) {
	if pl.pods == nil {
		return nil, false
	}
	p, ok := pl.pods[id]
	return p, ok
}

func (pl *PodList) Put(p *Pod) {
	if pl.pods == nil {
		pl.pods = make(map[string]*Pod)
	}
	pl.pods[p.id] = p
}

func (pl *PodList) Delete(id string) {
	delete(pl.pods, id)
}

func (pl *PodList) GetByName(name string) *Pod {
	return pl.Find(func(p *Pod) bool {
		if p.status.Name == name {
			return true
		}
		return false
	})
}

func (pl *PodList) GetStatus(id string) (*hypervisor.PodStatus, bool) {
	p, ok := pl.Get(id)
	if !ok {
		return nil, false
	}
	return p.status, true
}

func (pl *PodList) CountRunning() int64 {
	return pl.CountStatus(types.S_POD_RUNNING)
}

func (pl *PodList) CountStatus(status uint) (num int64) {
	num = 0

	pl.RLock()
	defer pl.RUnlock()

	if pl.pods == nil {
		return
	}

	for _, pod := range pl.pods {
		if pod.status.Status == status {
			num++
		}
	}

	return
}

func (pl *PodList) CountContainers() (num int64) {
	num = 0
	pl.RLock()
	defer pl.RUnlock()

	if pl.pods == nil {
		return
	}

	for _, pod := range pl.pods {
		num += int64(len(pod.status.Containers))
	}

	return
}

type PodOp func(*Pod) error
type PodFilterOp func(*Pod) bool

func (pl *PodList) Foreach(fn PodOp) error {
	for _, p := range pl.pods {
		if err := fn(p); err != nil {
			return err
		}
	}
	return nil
}

func (pl *PodList) Find(fn PodFilterOp) *Pod {
	for _, p := range pl.pods {
		if fn(p) {
			return p
		}
	}
	return nil
}
