package daemondb

import (
	"strings"

	"github.com/golang/glog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type DaemonDB struct {
	db *leveldb.DB
}

type KVPair struct {
	K []byte
	V []byte
}

func NewDaemonDB(db_file string) (*DaemonDB, error) {
	db, err := leveldb.OpenFile(db_file, nil)
	if err != nil {
		glog.Errorf("open leveldb file failed, %s", err.Error())
		return nil, err
	}
	return &DaemonDB{db: db}, nil
}

// Composition Process
func (d *DaemonDB) LagecyDeleteVMByPod(id string) error {
	v, err := d.LagecyGetP2V(id)
	if err != nil {
		return err
	}
	d.LagecyDeleteP2V(id)
	if err := d.LagecyDeleteVM(v); err != nil {
		return err
	}
	return nil
}

// Pods podId and args
func (d *DaemonDB) LagecyGetPod(id string) ([]byte, error) {
	return d.Get(keyPod(id))
}

func (d *DaemonDB) LagecyUpdatePod(id string, data []byte) error {
	return d.Update(keyPod(id), data)
}

func (d *DaemonDB) LagecyDeletePod(id string) error {
	return d.db.Delete(keyPod(id), nil)
}

func (d *DaemonDB) LagecyListPod() ([][]byte, error) {
	return d.PrefixListKey(prefixPod(), func(key []byte) bool {
		return !strings.HasPrefix(string(key), POD_CONTAINER_PREFIX)
	})
}

func (d *DaemonDB) LagecyGetAllPods() chan *KVPair {
	return d.PrefixList2Chan(prefixPod(), func(key []byte) bool {
		return !strings.HasPrefix(string(key), POD_CONTAINER_PREFIX)
	})
}

// Pod Volumes
func (d *DaemonDB) UpdatePodVolume(podId, volname string, data []byte) error {
	return d.Update(keyVolume(podId, volname), data)
}

func (d *DaemonDB) GetPodVolume(podId, volname string) ([]byte, error) {
	return d.db.Get(keyVolume(podId, volname), nil)
}

func (d *DaemonDB) ListPodVolumes(podId string) ([][]byte, error) {
	return d.PrefixList(prefixVolume(podId), nil)
}

func (d *DaemonDB) DeletePodVolume(podId, volName string) error {
	return d.db.Delete(keyVolume(podId, volName), nil)
}

func (d *DaemonDB) DeletePodVolumes(podId string) error {
	return d.PrefixDelete(prefixVolume(podId))
}

// POD to Containers (string to string list)
func (d *DaemonDB) LagecyGetP2C(id string) ([]string, error) {
	glog.V(3).Info("try get container list for pod ", id)
	cl, err := d.Get(keyP2C(id))
	if err != nil || len(cl) == 0 {
		return []string{}, err
	}
	return strings.Split(string(cl), ":"), nil
}

func (d *DaemonDB) LagecyUpdateP2C(id string, containers []string) error {
	glog.V(3).Infof("try set container list for pod %s: %v", id, containers)
	return d.Update(keyP2C(id), []byte(strings.Join(containers, ":")))
}

func (d *DaemonDB) LagecyDeleteP2C(id string) error {
	return d.db.Delete(keyP2C(id), nil)
}

// POD to VM (string to string)
func (d *DaemonDB) LagecyGetP2V(id string) (string, error) {
	return d.GetString(keyP2V(id))
}

func (d *DaemonDB) LagecyUpdateP2V(id, vm string) error {
	return d.Update(keyP2V(id), []byte(vm))
}

func (d *DaemonDB) LagecyDeleteP2V(id string) error {
	return d.db.Delete(keyP2V(id), nil)
}

func (d *DaemonDB) LagecyDeleteAllP2V() error {
	return d.PrefixDelete(prefixP2V())
}

// VM DATA (string to data)
func (d *DaemonDB) LagecyGetVM(id string) ([]byte, error) {
	data, err := d.db.Get(keyVMData(id), nil)
	if err != nil {
		return []byte(""), err
	}
	return data, nil
}

func (d *DaemonDB) LagecyUpdateVM(id string, data []byte) error {
	return d.Update(keyVMData(id), data)
}

func (d *DaemonDB) LagecyDeleteVM(id string) error {
	return d.db.Delete(keyVMData(id), nil)
}

// Low level util
func (d *DaemonDB) Close() error {
	return d.db.Close()
}

func (d *DaemonDB) Delete(key []byte) error {
	return d.db.Delete(key, nil)
}

func (d *DaemonDB) Get(key []byte) ([]byte, error) {
	data, err := d.db.Get(key, nil)
	if err != nil {
		return []byte(""), err
	}
	return data, nil
}

func (d *DaemonDB) GetString(key []byte) (string, error) {
	data, err := d.db.Get(key, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (d *DaemonDB) Update(key, data []byte) error {
	_, err := d.db.Get(key, nil)
	if err == nil {
		err = d.db.Delete(key, nil)
		if err != nil {
			return err
		}
	} else if err != leveldb.ErrNotFound {
		return err
	}

	err = d.db.Put(key, data, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *DaemonDB) PrefixDelete(prefix []byte) error {
	iter := d.db.NewIterator(util.BytesPrefix(prefix), nil)
	for iter.Next() {
		key := iter.Key()
		d.db.Delete(key, nil)
	}
	iter.Release()
	err := iter.Error()
	return err
}

type KeyFilter func([]byte) bool

func (d *DaemonDB) PrefixList(prefix []byte, keyFilter KeyFilter) ([][]byte, error) {
	var results [][]byte
	iter := d.db.NewIterator(util.BytesPrefix(prefix), nil)
	for iter.Next() {
		if keyFilter == nil || keyFilter(iter.Key()) {
			results = append(results, append([]byte{}, iter.Value()...))
		}
	}
	iter.Release()
	err := iter.Error()
	return results, err
}

func (d *DaemonDB) PrefixListKey(prefix []byte, keyFilter KeyFilter) ([][]byte, error) {
	var results [][]byte
	iter := d.db.NewIterator(util.BytesPrefix(prefix), nil)
	for iter.Next() {
		if keyFilter == nil || keyFilter(iter.Key()) {
			results = append(results, append([]byte{}, iter.Key()...))
		}
	}
	iter.Release()
	err := iter.Error()
	return results, err
}

func (d *DaemonDB) PrefixList2Chan(prefix []byte, keyFilter KeyFilter) chan *KVPair {
	ch := make(chan *KVPair, 128)
	if ch == nil {
		return ch
	}
	go func() {
		iter := d.db.NewIterator(util.BytesPrefix(prefix), nil)
		for iter.Next() {
			glog.V(3).Infof("got key from leveldb %s", string(iter.Key()))
			if keyFilter == nil || keyFilter(iter.Key()) {
				k := make([]byte, len(iter.Key()))
				v := make([]byte, len(iter.Value()))
				copy(k, iter.Key())
				copy(v, iter.Value())
				ch <- &KVPair{k, v}
			}
		}
		iter.Release()
		if err := iter.Error(); err != nil {
			ch <- nil
			glog.Error("Error occurs while iterate db with %v", prefix)
		}
		close(ch)
	}()

	return ch
}
