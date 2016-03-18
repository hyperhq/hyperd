package daemon

import (
	"strings"
	"sync"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

type PodList struct {
	pods       map[string]*Pod
	containers map[string]string
	sync.RWMutex
}

func NewPodList() *PodList {
	return &PodList{
		pods:       make(map[string]*Pod),
		containers: make(map[string]string),
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

	if pl.containers == nil {
		pl.containers = make(map[string]string)
	}
	for _, c := range p.status.Containers {
		pl.containers[c.Id] = p.id
	}
}

func (pl *PodList) Delete(id string) {
	if p, ok := pl.pods[id]; ok {
		for _, c := range p.status.Containers {
			delete(pl.containers, c.Id)
		}
	}
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

func (pl *PodList) GetByContainerId(cid string) (*Pod, bool) {
	if pl.pods == nil {
		return nil, false
	}
	if podid, ok := pl.containers[cid]; ok {
		p, ok := pl.pods[podid]
		return p, ok
	}

	pod := pl.Find(func(p *Pod) bool {
		for _, c := range p.status.Containers {
			if c.Id == cid {
				return true
			}
		}
		return false
	})

	if pod != nil {
		pl.containers[cid] = pod.id
		return pod, true
	}
	return nil, false
}

func (pl *PodList) GetByContainerIdOrName(cid string) (*Pod, int, bool) {
	if pl.pods == nil {
		return nil, 0, false
	}
	if podid, ok := pl.containers[cid]; ok {
		if p, ok := pl.pods[podid]; ok {
			for idx, c := range p.status.Containers {
				if c.Id == cid {
					return p, idx, true
				}
			}
		}
		return nil, -1, false
	}

	matchPods := []string{}
	fullId := ""
	for c, p := range pl.containers {
		if strings.HasPrefix(c, cid) {
			matchPods = append(matchPods, p)
			fullId = c
		}
	}
	if len(matchPods) > 1 {
		return nil, -1, false
	} else if len(matchPods) == 1 {
		if p, ok := pl.pods[matchPods[0]]; ok {
			for idx, c := range p.status.Containers {
				if c.Id == fullId {
					return p, idx, true
				}
			}
		}
		return nil, -1, false
	}

	var idx int
	wslash := cid
	if cid[0] != '/' {
		wslash = "/" + cid
	}

	pod := pl.Find(func(p *Pod) bool {
		for i, c := range p.status.Containers {
			if c.Id == cid || c.Name == wslash {
				idx = i
				return true
			}
		}
		return false
	})

	if pod != nil {
		return pod, idx, true
	}
	return nil, -1, false
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
