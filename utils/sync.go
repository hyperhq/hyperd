package utils

import (
	"fmt"
	"sync"
	"time"
)

type Initializer struct {
	once sync.Once
	job  func()
}

func NewInitializer(fn func()) *Initializer {
	return &Initializer{
		job: fn,
	}
}

func (i *Initializer) Do() {
	i.once.Do(i.job)
}

type WaitGroupWithFail struct {
	sync.WaitGroup
	sync.Mutex

	errs []error
}

func (wg *WaitGroupWithFail) Wait() error {
	wg.WaitGroup.Wait()
	if len(wg.errs) > 0 {
		return fmt.Errorf("Errors: %v", wg.errs)
	}
	return nil
}

func (wg *WaitGroupWithFail) Fail(err error) {
	wg.Mutex.Lock()
	if wg.errs == nil {
		wg.errs = []error{}
	}
	wg.errs = append(wg.errs, err)
	wg.Mutex.Unlock()
	wg.WaitGroup.Done()
}

type FutureSet struct {
	waitList   map[string]bool
	resultChan chan struct {
		id  string
		err error
	}
	sync.Mutex
}

var (
	ErrTimeout = fmt.Errorf("timeout")
	BrokenChan = fmt.Errorf("Unexpected broken chan")
)

func NewFutureSet() *FutureSet {
	return &FutureSet{
		waitList: make(map[string]bool),
		resultChan: make(chan struct {
			id  string
			err error
		}, 1),
	}
}

func (fs *FutureSet) IsFinished() bool {
	return len(fs.waitList) == 0
}

func (fs *FutureSet) Add(id string, op func() error) {
	for fs.waitList[id] {
		id = id + RandStr(4, "alphanum")
	}

	fs.Lock()
	fs.waitList[id] = true
	fs.Unlock()

	go func() {
		err := op()
		fs.resultChan <- struct {
			id  string
			err error
		}{id, err}
	}()
}

func (fs *FutureSet) Wait(timeout time.Duration) error {
	var toc <-chan time.Time
	if int64(timeout) < 0 {
		toc = make(chan time.Time)
	} else {
		toc = time.After(timeout)
	}

	errs := map[string]error{}
	for len(fs.waitList) > 0 {
		select {
		case r, ok := <-fs.resultChan:
			if !ok {
				return BrokenChan
			}
			fs.Lock()
			delete(fs.waitList, r.id)
			fs.Unlock()
			if r.err != nil {
				errs[r.id] = r.err
			}
		case <-toc:
			return ErrTimeout
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("finished with errors: %#v", errs)
	}

	return nil
}
