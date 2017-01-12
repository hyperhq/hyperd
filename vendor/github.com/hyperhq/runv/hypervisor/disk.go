package hypervisor

import (
	"sync"

	"github.com/hyperhq/runv/api"
	"strconv"
	"strings"
)

type DiskDescriptor struct {
	Name         string
	Filename     string
	Format       string
	Fstype       string
	DeviceName   string
	ScsiId       int
	ScsiAddr     string
	DockerVolume bool
	Options      map[string]string
}

func (d *DiskDescriptor) IsDir() bool {
	return d.Format == "vfs"
}

type DiskContext struct {
	*DiskDescriptor

	isRootVol bool
	sandbox   *VmContext
	ready     bool
	observers map[string]*sync.WaitGroup
	lock      *sync.RWMutex
}

func NewDiskContext(ctx *VmContext, vol *api.VolumeDescription) *DiskContext {
	dc := &DiskContext{
		DiskDescriptor: &DiskDescriptor{
			Name:         vol.Name,
			Filename:     vol.Source,
			Format:       vol.Format,
			Fstype:       vol.Fstype,
			DockerVolume: vol.DockerVolume,
		},
		sandbox:   ctx,
		observers: make(map[string]*sync.WaitGroup),
		lock:      &sync.RWMutex{},
	}
	if vol.IsDir() {
		dc.ready = true
	} else if vol.Format == "rbd" {
		dc.Options = map[string]string{
			"user":        vol.Options.User,
			"keyring":     vol.Options.Keyring,
			"monitors":    strings.Join(vol.Options.Monitors, ";"),
			"bytespersec": strconv.Itoa(int(vol.Options.BytesPerSec)),
			"iops":        strconv.Itoa(int(vol.Options.Iops)),
		}
	}
	return dc
}

func (dc *DiskContext) insert(result chan api.Result) {
	if result == nil {
		result = make(chan api.Result, 4)
	}
	if dc.isReady() {
		result <- api.NewResultBase(dc.Name, true, "already inserted")
		return
	}

	dc.ScsiId = dc.sandbox.nextScsiId()
	usage := "volume"
	if dc.isRootVol {
		usage = "image"
	}

	go func() {
		r := make(chan VmEvent, 4)
		dc.sandbox.DCtx.AddDisk(dc.sandbox, usage, dc.DiskDescriptor, r)

		ev, ok := <-r
		if !ok {
			dc.failed()
			result <- api.NewResultBase(dc.Name, false, "disk insert session broken")
			return
		}

		de, ok := ev.(*BlockdevInsertedEvent)
		if !ok {
			dc.failed()
			result <- api.NewResultBase(dc.Name, false, "disk insert failed")
			return
		}

		dc.DeviceName = de.DeviceName
		dc.ScsiAddr = de.ScsiAddr

		result <- api.NewResultBase(dc.Name, true, "")
		dc.inserted()
	}()
}

func (dc *DiskContext) remove(result chan<- api.Result) {
	if result == nil {
		result = make(chan api.Result, 4)
	}

	if dc.IsDir() {
		result <- api.NewResultBase(dc.Name, true, "no need to unplug")
		return
	}

	go func() {
		r := make(chan VmEvent, 4)
		dc.sandbox.DCtx.RemoveDisk(dc.sandbox, dc.DiskDescriptor, &VolumeUnmounted{Name: dc.Name, Success: true}, r)

		ev, ok := <-r
		if !ok {
			dc.failed()
			result <- api.NewResultBase(dc.Name, false, "disk remove session broken")
			return
		}

		_, ok = ev.(*VolumeUnmounted)
		if !ok {
			dc.failed()
			result <- api.NewResultBase(dc.Name, false, "disk remove failed")
			return
		}

		result <- api.NewResultBase(dc.Name, true, "")
	}()
}

func (dc *DiskContext) wait(id string, wg *sync.WaitGroup) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	if _, ok := dc.observers[id]; ok {
		return
	}

	dc.observers[id] = wg
	if dc.ready {
		return
	}
	wg.Add(1)
}

func (dc *DiskContext) unwait(id string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	if _, ok := dc.observers[id]; ok {
		dc.sandbox.Log(DEBUG, "container %s unwait disk %s", id, dc.Name)
		delete(dc.observers, id)
	}
}

func (dc *DiskContext) inserted() {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.ready = true
	for c, wg := range dc.observers {
		dc.sandbox.Log(INFO, "disk %s for container %s inserted", dc.Name, c)
		wg.Done()
	}
}

func (dc *DiskContext) failed() {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.ready = false
	for c, wg := range dc.observers {
		dc.sandbox.Log(INFO, "disk %s for container %s inserted", dc.Name, c)
		wg.Done()
	}
}

func (dc *DiskContext) isReady() bool {
	dc.lock.RLock()
	defer dc.lock.RUnlock()
	return dc.ready
}

func (dc *DiskContext) containers() int {
	dc.lock.RLock()
	defer dc.lock.RUnlock()
	return len(dc.observers)
}
