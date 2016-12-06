package daemon

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/daemon/daemondb"
	"github.com/hyperhq/hyperd/storage"
	"github.com/hyperhq/hyperd/storage/aufs"
	dm "github.com/hyperhq/hyperd/storage/devicemapper"
	"github.com/hyperhq/hyperd/storage/overlay"
	"github.com/hyperhq/hyperd/storage/vbox"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
)

type Storage interface {
	Type() string
	RootPath() string

	Init() error
	CleanUp() error

	PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error)
	CleanupContainer(id, sharedDir string) error
	InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error
	CreateVolume(podId string, spec *apitypes.UserVolume) error
	RemoveVolume(podId string, record []byte) error
}

var StorageDrivers map[string]func(*dockertypes.Info, *daemondb.DaemonDB) (Storage, error) = map[string]func(*dockertypes.Info, *daemondb.DaemonDB) (Storage, error){
	"devicemapper": DMFactory,
	"aufs":         AufsFactory,
	"overlay":      OverlayFsFactory,
	"vbox":         VBoxStorageFactory,
}

func StorageFactory(sysinfo *dockertypes.Info, db *daemondb.DaemonDB) (Storage, error) {
	if factory, ok := StorageDrivers[sysinfo.Driver]; ok {
		return factory(sysinfo, db)
	}
	return nil, fmt.Errorf("hyperd can not support docker's backing storage: %s", sysinfo.Driver)
}

type DevMapperStorage struct {
	db          *daemondb.DaemonDB
	CtnPoolName string
	VolPoolName string
	DevPrefix   string
	FsType      string
	rootPath    string
	DmPoolData  *dm.DeviceMapper
}

func DMFactory(sysinfo *dockertypes.Info, db *daemondb.DaemonDB) (Storage, error) {
	driver := &DevMapperStorage{
		db: db,
	}

	driver.VolPoolName = storage.DEFAULT_DM_POOL

	for _, pair := range sysinfo.DriverStatus {
		if pair[0] == "Pool Name" {
			driver.CtnPoolName = pair[1]
		}
		if pair[0] == "Backing Filesystem" {
			if strings.Contains(pair[1], "ext") {
				driver.FsType = "ext4"
			} else if strings.Contains(pair[1], "xfs") {
				driver.FsType = "xfs"
			} else {
				driver.FsType = "dir"
			}
			break
		}
	}
	driver.DevPrefix = driver.CtnPoolName[:strings.Index(driver.CtnPoolName, "-pool")]
	driver.rootPath = filepath.Join(utils.HYPER_ROOT, "devicemapper")
	return driver, nil
}

func (dms *DevMapperStorage) Type() string {
	return "devicemapper"
}

func (dms *DevMapperStorage) RootPath() string {
	return dms.rootPath
}

func (dms *DevMapperStorage) Init() error {
	dmPool := dm.DeviceMapper{
		Datafile:         filepath.Join(utils.HYPER_ROOT, "lib") + "/data",
		Metadatafile:     filepath.Join(utils.HYPER_ROOT, "lib") + "/metadata",
		DataLoopFile:     storage.DEFAULT_DM_DATA_LOOP,
		MetadataLoopFile: storage.DEFAULT_DM_META_LOOP,
		PoolName:         dms.VolPoolName,
		Size:             storage.DEFAULT_DM_POOL_SIZE,
	}
	dms.DmPoolData = &dmPool
	rand.Seed(time.Now().UnixNano())

	// Prepare the DeviceMapper storage
	return dm.CreatePool(&dmPool)
}

func (dms *DevMapperStorage) CleanUp() error {
	return dm.DMCleanup(dms.DmPoolData)
}

func (dms *DevMapperStorage) PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error) {
	if err := dm.CreateNewDevice(mountId, dms.DevPrefix, dms.RootPath()); err != nil {
		return nil, err
	}
	devFullName, err := dm.MountContainerToSharedDir(mountId, sharedDir, dms.DevPrefix)
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}
	fstype, err := dm.ProbeFsType(devFullName)
	if err != nil {
		fstype = storage.DEFAULT_VOL_FS
	}

	vol := &runv.VolumeDescription{
		Name:   devFullName,
		Source: devFullName,
		Fstype: fstype,
		Format: "raw",
	}

	return vol, nil
}

func (dms *DevMapperStorage) CleanupContainer(id, sharedDir string) error {
	devFullName, err := dm.MountContainerToSharedDir(id, sharedDir, dms.DevPrefix)
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return err
	}

	return dm.UnmapVolume(devFullName)
}

func (dms *DevMapperStorage) InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error {
	return dm.InjectFile(src, containerId, dms.DevPrefix, target, baseDir, perm, uid, gid)
}

func (dms *DevMapperStorage) getPersistedId(podId, volName string) (int, error) {
	vols, err := dms.db.ListPodVolumes(podId)
	if err != nil {
		return -1, err
	}

	dev_id := 0
	for _, vol := range vols {
		fields := strings.Split(string(vol), ":")
		if fields[0] == volName {
			dev_id, _ = strconv.Atoi(fields[1])
		}
	}
	return dev_id, nil
}

func (dms *DevMapperStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	var err error

	deviceName := fmt.Sprintf("%s-%s-%s", dms.VolPoolName, podId, spec.Name)
	dev_id, _ := dms.getPersistedId(podId, deviceName)
	glog.Infof("DeviceID is %d", dev_id)

	restore := dev_id > 0

	for {
		if !restore {
			dev_id = dms.randDevId()
		}
		dev_id_str := strconv.Itoa(dev_id)

		err = dm.CreateVolume(dms.VolPoolName, deviceName, dev_id_str, storage.DEFAULT_VOL_MKFS, storage.DEFAULT_DM_VOL_SIZE, restore)
		if err != nil && !restore && strings.Contains(err.Error(), "failed: File exists") {
			glog.V(1).Infof("retry for dev_id #%d creating collision: %v", dev_id, err)
			continue
		} else if err != nil {
			glog.V(1).Infof("failed to create dev_id #%d: %v", dev_id, err)
			return err
		}

		glog.V(3).Infof("device (%d) created (restore:%v) for %s: %s", dev_id, restore, podId, deviceName)
		dms.db.UpdatePodVolume(podId, deviceName, []byte(fmt.Sprintf("%s:%s", deviceName, dev_id_str)))
		break
	}

	fstype := storage.DEFAULT_VOL_FS
	if !restore {
		if spec.Fstype == "" {
			fstype, err = dm.ProbeFsType("/dev/mapper/" + deviceName)
			if err != nil {
				fstype = storage.DEFAULT_VOL_FS
			}
		} else {
			fstype = spec.Fstype
		}
	}

	glog.V(1).Infof("volume %s created with dm as %s", spec.Name, deviceName)

	spec.Source = filepath.Join("/dev/mapper/", deviceName)
	spec.Format = "raw"
	spec.Fstype = fstype

	return nil
}

func (dms *DevMapperStorage) RemoveVolume(podId string, record []byte) error {
	fields := strings.SplitN(string(record), ":", 2)
	if len(fields) == 1 {
		record, err := dms.db.GetPodVolume(podId, fields[0])
		if err != nil {
			glog.Error(err)
			return err
		}
		fields = strings.SplitN(string(record), ":", 2)
		if len(fields) == 1 {
			err = fmt.Errorf("cannot get valid volume %s/%s from db", podId, record)
			glog.Error(err)
			return err
		}
	}
	dev_id, _ := strconv.Atoi(fields[1])
	if err := dm.DeleteVolume(dms.DmPoolData, dev_id); err != nil {
		glog.Error(err.Error())
		return err
	}
	if err := dms.db.DeletePodVolume(podId, fields[0]); err != nil {
		glog.Error(err.Error())
		return err
	}
	return nil
}

func (dms *DevMapperStorage) randDevId() int {
	return rand.Intn(1<<24-1) + 1 // 0 reserved for pool device
}

type AufsStorage struct {
	rootPath string
}

func AufsFactory(sysinfo *dockertypes.Info, _ *daemondb.DaemonDB) (Storage, error) {
	driver := &AufsStorage{}
	for _, pair := range sysinfo.DriverStatus {
		if pair[0] == "Root Dir" {
			driver.rootPath = pair[1]
		}
	}
	return driver, nil
}

func (a *AufsStorage) Type() string {
	return "aufs"
}

func (a *AufsStorage) RootPath() string {
	return a.rootPath
}

func (*AufsStorage) Init() error { return nil }

func (*AufsStorage) CleanUp() error { return nil }

func (a *AufsStorage) PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error) {
	_, err := aufs.MountContainerToSharedDir(mountId, a.RootPath(), sharedDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	containerPath := "/" + mountId
	vol := &runv.VolumeDescription{
		Name:   containerPath,
		Source: containerPath,
		Fstype: "dir",
		Format: "vfs",
	}

	return vol, nil
}

func (a *AufsStorage) CleanupContainer(id, sharedDir string) error {
	return aufs.Unmount(filepath.Join(sharedDir, id, "rootfs"))
}

func (a *AufsStorage) InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error {
	_, err := aufs.MountContainerToSharedDir(containerId, a.RootPath(), baseDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return err
	}
	defer aufs.Unmount(filepath.Join(baseDir, containerId, "rootfs"))

	return storage.FsInjectFile(src, containerId, target, baseDir, perm, uid, gid)
}

func (a *AufsStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	volName, err := storage.CreateVFSVolume(podId, spec.Name)
	if err != nil {
		return err
	}
	spec.Source = volName
	spec.Format = "vfs"
	spec.Fstype = "dir"
	return nil
}

func (a *AufsStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}

type OverlayFsStorage struct {
	rootPath string
}

func OverlayFsFactory(_ *dockertypes.Info, _ *daemondb.DaemonDB) (Storage, error) {
	driver := &OverlayFsStorage{
		rootPath: filepath.Join(utils.HYPER_ROOT, "overlay"),
	}
	return driver, nil
}

func (o *OverlayFsStorage) Type() string {
	return "overlay"
}

func (o *OverlayFsStorage) RootPath() string {
	return o.rootPath
}

func (*OverlayFsStorage) Init() error { return nil }

func (*OverlayFsStorage) CleanUp() error { return nil }

func (o *OverlayFsStorage) PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error) {
	_, err := overlay.MountContainerToSharedDir(mountId, o.RootPath(), sharedDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	containerPath := "/" + mountId
	vol := &runv.VolumeDescription{
		Name:   containerPath,
		Source: containerPath,
		Fstype: "dir",
		Format: "vfs",
	}

	return vol, nil
}

func (o *OverlayFsStorage) CleanupContainer(id, sharedDir string) error {
	return syscall.Unmount(filepath.Join(sharedDir, id, "rootfs"), 0)
}

func (o *OverlayFsStorage) InjectFile(src io.Reader, mountId, target, baseDir string, perm, uid, gid int) error {
	_, err := overlay.MountContainerToSharedDir(mountId, o.RootPath(), baseDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return err
	}
	defer syscall.Unmount(filepath.Join(baseDir, mountId, "rootfs"), 0)

	return storage.FsInjectFile(src, mountId, target, baseDir, perm, uid, gid)
}

func (o *OverlayFsStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	volName, err := storage.CreateVFSVolume(podId, spec.Name)
	if err != nil {
		return err
	}
	spec.Source = volName
	spec.Format = "vfs"
	spec.Fstype = "dir"
	return nil
}

func (o *OverlayFsStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}

type VBoxStorage struct {
	rootPath string
}

func VBoxStorageFactory(_ *dockertypes.Info, _ *daemondb.DaemonDB) (Storage, error) {
	driver := &VBoxStorage{
		rootPath: filepath.Join(utils.HYPER_ROOT, "vbox"),
	}
	return driver, nil
}

func (v *VBoxStorage) Type() string {
	return "vbox"
}

func (v *VBoxStorage) RootPath() string {
	return v.rootPath
}

func (*VBoxStorage) Init() error { return nil }

func (*VBoxStorage) CleanUp() error { return nil }

func (v *VBoxStorage) PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error) {
	devFullName, err := vbox.MountContainerToSharedDir(mountId, v.RootPath(), "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	vol := &runv.VolumeDescription{
		Name:   devFullName,
		Source: devFullName,
		Fstype: "ext4",
		Format: "vdi",
	}

	return vol, nil
}

func (v *VBoxStorage) CleanupContainer(id, sharedDir string) error {
	return nil
}

func (v *VBoxStorage) InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	return errors.New("vbox storage driver does not support file insert yet")
}

func (v *VBoxStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	volName, err := storage.CreateVFSVolume(podId, spec.Name)
	if err != nil {
		return err
	}
	spec.Source = volName
	spec.Format = "vfs"
	spec.Fstype = "dir"
	return nil
}

func (v *VBoxStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}
