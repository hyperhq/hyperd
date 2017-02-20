// +build linux

package vsock

import (
	"fmt"
	"sync"

	"github.com/RoaringBitmap/roaring"
)

const hyperDefaultVsockCid = 1024
const hyperDefaultVsockBitmapSize = 16384

type VsockCidAllocator interface {
	sync.Locker
	GetCid() (uint32, error)
	MarkCidInuse(uint32) bool
	ReleaseCid(uint32)
}

type DefaultVsockCidAllocator struct {
	sync.Mutex
	bitmap *roaring.Bitmap
	start  uint32
	size   uint32
	pivot  uint32
}

func NewDefaultVsockCidAllocator() VsockCidAllocator {
	return &DefaultVsockCidAllocator{
		bitmap: roaring.NewBitmap(),
		start:  hyperDefaultVsockCid,
		size:   hyperDefaultVsockBitmapSize,
		pivot:  hyperDefaultVsockCid,
	}
}

func (vc *DefaultVsockCidAllocator) GetCid() (uint32, error) {
	var cid uint32
	vc.Lock()
	defer vc.Unlock()
	for i := uint32(0); i < vc.size; i++ {
		cid = vc.pivot + i
		if cid >= vc.start+vc.size {
			cid -= vc.size
		}
		if vc.bitmap.CheckedAdd(cid) {
			vc.pivot = cid + 1
			return cid, nil
		}
	}

	return cid, fmt.Errorf("No more available cid")
}

func (vc *DefaultVsockCidAllocator) MarkCidInuse(cid uint32) bool {
	vc.Lock()
	defer vc.Unlock()
	success := vc.bitmap.CheckedAdd(cid)
	return success
}

func (vc *DefaultVsockCidAllocator) ReleaseCid(cid uint32) {
	vc.Lock()
	defer vc.Unlock()
	vc.bitmap.Remove(cid)
}
