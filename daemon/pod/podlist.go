package pod

import (
	"fmt"
	"strings"
	"sync"
)

type PodList struct {
	pods           map[string]*XPod
	containers     map[string]string
	containerNames map[string]string
	mu             *sync.RWMutex
}

func NewPodList() *PodList {
	return &PodList{
		pods:           make(map[string]*XPod),
		containers:     make(map[string]string),
		containerNames: make(map[string]string),
		mu:             &sync.RWMutex{},
	}
}

func (pl *PodList) Get(id string) (*XPod, bool) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	p, ok := pl.pods[id]
	return p, ok
}

func (pl *PodList) ReserveContainerID(id, pod string) error {
	if pn, ok := pl.containers[id]; ok && pn != pod {
		return fmt.Errorf("the container id %s has already taken by pod %s", id, pn)
	}
	pl.containers[id] = pod
	return nil
}

func (pl *PodList) ReserveContainerName(name, pod string) error {
	if pn, ok := pl.containerNames[name]; ok && pn != pod {
		return fmt.Errorf("the container name %s has already taken by pod %s", name, pn)
	}
	pl.containerNames[name] = pod
	return nil
}

func (pl *PodList) ReserveContainer(id, name, pod string) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if _, ok := pl.pods[pod]; !ok {
		return fmt.Errorf("pod %s not exist for adding container %s(%s)", pod, name, id)
	}
	if pn, ok := pl.containerNames[name]; ok && pn != pod {
		return fmt.Errorf("container name %s has already taken by pod %s", name, pn)
	}
	if id != "" {
		if pn, ok := pl.containers[id]; ok && pn != pod {
			return fmt.Errorf("the container id %s has already taken by pod %s", id, pn)
		}
		pl.containers[id] = pod
	}
	pl.containerNames[name] = pod
	return nil
}

func (pl *PodList) ReservePod(p *XPod) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	name := p.Id()
	// check availability
	if pe, ok := pl.pods[name]; ok && pe != p {
		return fmt.Errorf("pod name %s has already in use", p.Id())
	}

	pl.pods[name] = p
	return nil
}

func (pl *PodList) Release(id string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if p, ok := pl.pods[id]; ok {
		for _, c := range p.ContainerIds() {
			delete(pl.containers, c)
		}
		for _, c := range p.ContainerNames() {
			delete(pl.containerNames, c)
		}
	}
	delete(pl.pods, id)
}

func (pl *PodList) ReleaseContainer(id, name string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	delete(pl.containers, id)
	delete(pl.containerNames, name)
}

func (pl *PodList) ReleaseContainerName(name string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	delete(pl.containerNames, name)
}

func (pl *PodList) GetByContainerId(cid string) (*XPod, bool) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	if podid, ok := pl.containers[cid]; ok {
		p, ok := pl.pods[podid]
		return p, ok
	}
	return nil, false
}

func (pl *PodList) GetByContainerIdOrName(cid string) (*XPod, string, bool) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	if podid, ok := pl.containerNames[cid]; ok {
		if p, ok := pl.pods[podid]; ok {
			id, _ := p.ContainerName2Id(cid)
			return p, id, true
		}
		return nil, "", false
	}
	if podid, ok := pl.containers[cid]; ok {
		if p, ok := pl.pods[podid]; ok {
			return p, cid, true
		}
		return nil, "", false
	}

	matchPods := []string{}
	fullid := ""
	for c, p := range pl.containers {
		if strings.HasPrefix(c, cid) {
			fullid = c
			matchPods = append(matchPods, p)
		}
	}
	if len(matchPods) > 1 {
		return nil, "", false
	} else if len(matchPods) == 1 {
		if p, ok := pl.pods[matchPods[0]]; ok {
			return p, fullid, true
		}
		return nil, "", false
	}

	return nil, "", false
}

func (pl *PodList) CountRunning() int64 {
	return pl.CountStatus(S_POD_RUNNING) + pl.CountStatus(S_POD_STARTING) + pl.CountStatus(S_POD_PAUSED)
}

func (pl *PodList) CountAll() int64 {
	return int64(len(pl.pods))
}

func (pl *PodList) CountStatus(status PodState) (num int64) {
	num = 0

	pl.mu.RLock()
	defer pl.mu.RUnlock()

	if pl.pods == nil {
		return
	}

	for _, pod := range pl.pods {
		if pod.status == status {
			num++
		}
	}

	return
}

func (pl *PodList) CountContainers() (num int64) {
	num = 0
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	return int64(len(pl.containers))
}

type PodOp func(*XPod) error
type PodFilterOp func(*XPod) bool

func (pl *PodList) Foreach(fn PodOp) error {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.foreachUnsafe(fn)
}

func (pl *PodList) foreachUnsafe(fn PodOp) error {
	for _, p := range pl.pods {
		if err := fn(p); err != nil {
			return err
		}
	}
	return nil
}

func (pl *PodList) Find(fn PodFilterOp) *XPod {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.findUnsafe(fn)
}

func (pl *PodList) findUnsafe(fn PodFilterOp) *XPod {
	for _, p := range pl.pods {
		if fn(p) {
			return p
		}
	}
	return nil
}
