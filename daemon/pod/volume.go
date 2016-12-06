package pod

import (
	"fmt"
	"sync"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
)

type VolumeState int32

const (
	S_VOLUME_CREATED VolumeState = iota
	S_VOLUME_IMSERTING
	S_VOLUME_INSERTED
	S_VOLUME_ERROR
)

type Volume struct {
	p *XPod

	spec     *apitypes.UserVolume
	descript *runv.VolumeDescription
	status   VolumeState

	insertSubscribers []*utils.WaitGroupWithFail

	sync.RWMutex
}

func newVolume(p *XPod, spec *apitypes.UserVolume) *Volume {
	return &Volume{
		p:                 p,
		spec:              spec,
		insertSubscribers: []*utils.WaitGroupWithFail{},
	}
}

func (v *Volume) LogPrefix() string {
	return fmt.Sprintf("%sVol[%s] ", v.p.LogPrefix(), v.spec.Name)
}

func (v *Volume) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, v, 1, args...)
}

func (v *Volume) getStatus() VolumeState {
	v.RLock()
	s := v.status
	v.RUnlock()
	return s
}

func (v *Volume) Info() *apitypes.PodVolume {
	return &apitypes.PodVolume{
		Name:   v.spec.Name,
		Source: v.spec.Source,
		Driver: v.spec.Format,
	}
}

// add() try to mount the volume and add it to the sandbox
func (v *Volume) add() error {
	changed, err := v.transit(
		S_VOLUME_IMSERTING,
		map[VolumeState]bool{S_VOLUME_CREATED: true},                            // from created to inserting
		map[VolumeState]bool{S_VOLUME_IMSERTING: true, S_VOLUME_INSERTED: true}, //ignore if inserting or inserted already
	)
	if !changed { // already logged in v.transit method
		return err
	}

	defer func() {
		if err != nil {
			v.setInsertFail(err)
		}
	}()

	err = v.mount()
	if err != nil {
		v.Log(ERROR, "failed to mount volume: %v", err)
		return err
	}
	defer func() {
		if err != nil {
			v.umount()
		}
	}()

	err = v.insert()
	if err != nil {
		v.Log(ERROR, "failed to mount volume: %v", err)
		return err
	}

	//only from inserting to inserted
	v.setInserted()
	return nil
}

// insert() should only called by add(), and not expose to outside
// the class.
func (v *Volume) insert() error {
	r := v.p.sandbox.AddVolume(v.descript)
	if !r.IsSuccess() {
		err := fmt.Errorf("failed to insert: %s", r.Message())
		v.Log(ERROR, err)
		return err
	}

	v.Log(INFO, "volume inserted")
	return nil
}

func (v *Volume) removeFromSandbox() error {
	removed, err := v.tryRemoveFromSandbox()
	if err != nil {
		return err
	}
	if !removed {
		err := fmt.Errorf("volume is in use, could not be removed")
		v.Log(ERROR, err)
		return err
	}
	v.Log(INFO, "volume removed from sandbox")
	return nil
}

func (v *Volume) tryRemoveFromSandbox() (bool, error) {
	var (
		removed bool
		err     error
	)
	r := v.p.sandbox.RemoveVolume(v.spec.Name)
	removed = r.IsSuccess()
	if !removed && (r.Message() != "in use") {
		err = fmt.Errorf("failed to remove vol from sandbox: %s", r.Message())
		v.Log(ERROR, err)
	}

	if removed {
		v.Lock()
		v.status = S_VOLUME_CREATED
		v.Unlock()
	}
	v.Log(INFO, "volume remove from sandbox (removed: %v)", removed)
	return removed, err
}

// mount() should only called by add(), and not expose to outside
// the class.
func (v *Volume) mount() error {
	var (
		err error
	)

	sharedDir := v.p.sandboxShareDir()
	v.descript, err = ProbeExistingVolume(v.spec, sharedDir)
	if err != nil {
		v.Log(ERROR, "volume probe/mount failed: %v", err)
		return err
	}

	return nil
}

func (v *Volume) umount() error {
	if v.descript != nil {
		return UmountExistingVolume(v.descript.Fstype, v.descript.Source, v.p.sandboxShareDir())
	}
	return nil
}

func (v *Volume) subscribeInsert(wg *utils.WaitGroupWithFail) error {
	v.Lock()
	defer v.Unlock()
	if v.status == S_VOLUME_INSERTED {
		return nil
	} else if v.status == S_VOLUME_ERROR {
		return fmt.Errorf("volume %s is in ERROR state", v.spec.Name)
	}
	wg.Add(1)
	v.insertSubscribers = append(v.insertSubscribers, wg)
	return nil
}

func (v *Volume) transit(to VolumeState, from, ignore map[VolumeState]bool) (changed bool, err error) {
	v.Lock()
	changed, err = v.unlockedTransit(to, from, ignore)
	v.Unlock()
	return changed, err
}

func (v *Volume) setInserted() {
	v.Lock()
	//only from inserting to inserted
	v.unlockedTransit(S_VOLUME_INSERTED, map[VolumeState]bool{S_VOLUME_IMSERTING: true}, map[VolumeState]bool{})
	if v.insertSubscribers != nil {
		for _, wg := range v.insertSubscribers {
			wg.Done()
		}
		v.insertSubscribers = []*utils.WaitGroupWithFail{}
	}
	v.Unlock()
}

func (v *Volume) setInsertFail(err error) {
	v.Lock()
	v.unlockedTransit(S_VOLUME_ERROR, map[VolumeState]bool{S_VOLUME_IMSERTING: true}, map[VolumeState]bool{})
	if v.insertSubscribers != nil {
		for _, wg := range v.insertSubscribers {
			wg.Fail(err)
		}
		v.insertSubscribers = []*utils.WaitGroupWithFail{}
	}
	v.Unlock()
}

func (v *Volume) unlockedTransit(to VolumeState, from, ignore map[VolumeState]bool) (bool, error) {
	if ignore[v.status] || v.status == to {
		v.Log(DEBUG, "does not transit volume from state %v to %v, ignored", v.status, to)
		return false, nil
	} else if from[v.status] {
		v.Log(DEBUG, "transit volume from state %v to %v, ok", v.status, to)
		v.status = to
		return true, nil
	}
	err := fmt.Errorf("cannot transit volume from state %v to %v", v.status, to)
	v.Log(ERROR, err)
	return false, err
}
