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
func (d *DaemonDB) DeleteVMByPod(id string) error {
	v, err := d.GetP2V(id)
	if err != nil {
		return err
	}
	d.DeleteP2V(id)
	if err := d.DeleteVM(v); err != nil {
		return err
	}
	return nil
}

// Pods podId and args
func (d *DaemonDB) GetPod(id string) ([]byte, error) {
	return d.Get(keyPod(id))
}

func (d *DaemonDB) UpdatePod(id string, data []byte) error {
	return d.Update(keyPod(id), data)
}

func (d *DaemonDB) DeletePod(id string) error {
	return d.db.Delete(keyPod(id), nil)
}

func (d *DaemonDB) ListPod() ([][]byte, error) {
	return d.PrefixListKey(prefixPod(), func(key []byte) bool {
		return !strings.HasPrefix(string(key), POD_CONTAINER_PREFIX)
	})
}

func (d *DaemonDB) GetAllPods() chan *KVPair {
	return d.PrefixList2Chan(prefixPod(), func(key []byte) bool {
		return !strings.HasPrefix(string(key), POD_CONTAINER_PREFIX)
	})
}

// Pod Volumes
func (d *DaemonDB) UpdatePodVolume(podId, volname string, data []byte) error {
	return d.Update(keyVolume(podId, volname), data)
}

func (d *DaemonDB) ListPodVolumes(podId string) ([][]byte, error) {
	return d.PrefixList(prefixVolume(podId), nil)
}

func (d *DaemonDB) DeletePodVolumes(podId string) error {
	return d.PrefixDelete(prefixVolume(podId))
}

// POD to Containers (string to string list)
func (d *DaemonDB) GetP2C(id string) ([]string, error) {
	glog.V(3).Info("try get container list for pod ", id)
	cl, err := d.Get(keyP2C(id))
	if err != nil || len(cl) == 0 {
		return []string{}, err
	}
	return strings.Split(string(cl), ":"), nil
}

func (d *DaemonDB) UpdateP2C(id string, containers []string) error {
	glog.V(3).Infof("try set container list for pod %s: %v", id, containers)
	return d.Update(keyP2C(id), []byte(strings.Join(containers, ":")))
}

func (d *DaemonDB) DeleteP2C(id string) error {
	return d.db.Delete(keyP2C(id), nil)
}

// POD to VM (string to string)
func (d *DaemonDB) GetP2V(id string) (string, error) {
	return d.GetString(keyP2V(id))
}

func (d *DaemonDB) UpdateP2V(id, vm string) error {
	return d.Update(keyP2V(id), []byte(vm))
}

func (d *DaemonDB) DeleteP2V(id string) error {
	return d.db.Delete(keyP2V(id), nil)
}

func (d *DaemonDB) DeleteAllP2V() error {
	return d.PrefixDelete(prefixP2V())
}

// VM DATA (string to data)
func (d *DaemonDB) GetVM(id string) ([]byte, error) {
	data, err := d.db.Get(keyVMData(id), nil)
	if err != nil {
		return []byte(""), err
	}
	return data, nil
}

func (d *DaemonDB) UpdateVM(id string, data []byte) error {
	return d.Update(keyVMData(id), data)
}

func (d *DaemonDB) DeleteVM(id string) error {
	return d.db.Delete(keyVMData(id), nil)
}

// Low level util
func (d *DaemonDB) Close() error {
	return d.db.Close()
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
				ch <- &KVPair{append([]byte{}, iter.Key()...), append([]byte{}, iter.Value()...)}
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
