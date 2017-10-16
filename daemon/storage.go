package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
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
	"github.com/hyperhq/hyperd/storage/graphdriver/rawblock"
	"github.com/hyperhq/hyperd/storage/overlay"
	"github.com/hyperhq/hyperd/storage/vbox"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
)

type Storage interface {
	Type() string
	RootPath() string

	Init(c *apitypes.HyperConfig) error
	CleanUp() error

	PrepareContainer(mountId, sharedDir string, readonly bool) (*runv.VolumeDescription, error)
	CleanupContainer(id, sharedDir string) error
	InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error
	CreateVolume(podId string, spec *apitypes.UserVolume) error
	RemoveVolume(podId string, record []byte) error
}

var StorageDrivers map[string]func(*dockertypes.Info, *daemondb.DaemonDB) (Storage, error) = map[string]func(*dockertypes.Info, *daemondb.DaemonDB) (Storage, error){
	"devicemapper": DMFactory,
	"aufs":         AufsFactory,
	"overlay":      OverlayFsFactory,
	"btrfs":        BtrfsFactory,
	"rawblock":     RawBlockFactory,
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

func (dms *DevMapperStorage) Init(c *apitypes.HyperConfig) error {
	size := storage.DEFAULT_DM_POOL_SIZE
	if c.StorageBaseSize > 0 {
		size = c.StorageBaseSize
	}
	dmPool := dm.DeviceMapper{
		Datafile:         filepath.Join(utils.HYPER_ROOT, "lib") + "/data",
		Metadatafile:     filepath.Join(utils.HYPER_ROOT, "lib") + "/metadata",
		DataLoopFile:     storage.DEFAULT_DM_DATA_LOOP,
		MetadataLoopFile: storage.DEFAULT_DM_META_LOOP,
		PoolName:         dms.VolPoolName,
		Size:             size,
	}
	dms.DmPoolData = &dmPool
	rand.Seed(time.Now().UnixNano())

	// Prepare the DeviceMapper storage
	return dm.CreatePool(&dmPool)
}

func (dms *DevMapperStorage) CleanUp() error {
	return dm.DMCleanup(dms.DmPoolData)
}

func (dms *DevMapperStorage) PrepareContainer(mountId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
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
		Name:     devFullName,
		Source:   devFullName,
		Fstype:   fstype,
		Format:   "raw",
		ReadOnly: readonly,
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

func (dms *DevMapperStorage) InjectFile(src io.Reader, mountId, target, baseDir string, perm, uid, gid int) error {
	if err := dm.CreateNewDevice(mountId, dms.DevPrefix, dms.RootPath()); err != nil {
		return err
	}
	return dm.InjectFile(src, mountId, dms.DevPrefix, target, baseDir, perm, uid, gid)
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

	// kernel dm has limitation of 128 bytes on device name length
	// include/uapi/linux/dm-ioctl.h#L16
	// #define DM_NAME_LEN 128
	// Use sha256 so it is fixed 64 bytes
	chksum := sha256.Sum256([]byte(podId + spec.Name))
	deviceName := fmt.Sprintf("%s-%s", dms.VolPoolName, hex.EncodeToString(chksum[:sha256.Size]))
	dev_id, _ := dms.getPersistedId(podId, deviceName)
	glog.Infof("DeviceID is %d for %s of pod %s container %s", dev_id, deviceName, podId, spec.Name)

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

func (*AufsStorage) Init(c *apitypes.HyperConfig) error { return nil }

func (*AufsStorage) CleanUp() error { return nil }

func (a *AufsStorage) PrepareContainer(mountId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
	_, err := aufs.MountContainerToSharedDir(mountId, a.RootPath(), sharedDir, "", readonly)
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	containerPath := "/" + mountId
	vol := &runv.VolumeDescription{
		Name:     containerPath,
		Source:   containerPath,
		Fstype:   "dir",
		Format:   "vfs",
		ReadOnly: readonly,
	}

	return vol, nil
}

func (a *AufsStorage) CleanupContainer(id, sharedDir string) error {
	return aufs.Unmount(filepath.Join(sharedDir, id, "rootfs"))
}

func (a *AufsStorage) InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error {
	_, err := aufs.MountContainerToSharedDir(containerId, a.RootPath(), baseDir, "", false)
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

func (*OverlayFsStorage) Init(c *apitypes.HyperConfig) error { return nil }

func (*OverlayFsStorage) CleanUp() error { return nil }

func (o *OverlayFsStorage) PrepareContainer(mountId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
	_, err := overlay.MountContainerToSharedDir(mountId, o.RootPath(), sharedDir, "", readonly)
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	containerPath := "/" + mountId
	vol := &runv.VolumeDescription{
		Name:     containerPath,
		Source:   containerPath,
		Fstype:   "dir",
		Format:   "vfs",
		ReadOnly: readonly,
	}

	return vol, nil
}

func (o *OverlayFsStorage) CleanupContainer(id, sharedDir string) error {
	return syscall.Unmount(filepath.Join(sharedDir, id, "rootfs"), 0)
}

func (o *OverlayFsStorage) InjectFile(src io.Reader, mountId, target, baseDir string, perm, uid, gid int) error {
	_, err := overlay.MountContainerToSharedDir(mountId, o.RootPath(), baseDir, "", false)
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

type BtrfsStorage struct {
	rootPath string
}

func BtrfsFactory(_ *dockertypes.Info, _ *daemondb.DaemonDB) (Storage, error) {
	driver := &BtrfsStorage{
		rootPath: filepath.Join(utils.HYPER_ROOT, "btrfs"),
	}
	return driver, nil
}

func (s *BtrfsStorage) Type() string {
	return "btrfs"
}

func (s *BtrfsStorage) RootPath() string {
	return s.rootPath
}

func (s *BtrfsStorage) subvolumesDirID(id string) string {
	return filepath.Join(s.RootPath(), "subvolumes", id)
}

func (*BtrfsStorage) Init(c *apitypes.HyperConfig) error { return nil }

func (*BtrfsStorage) CleanUp() error { return nil }

func (s *BtrfsStorage) PrepareContainer(containerId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
	btrfsRootfs := s.subvolumesDirID(containerId)
	mountPoint := filepath.Join(sharedDir, containerId, "rootfs")

	if _, err := os.Stat(mountPoint); err != nil {
		if err = os.MkdirAll(mountPoint, 0755); err != nil {
			return nil, err
		}
	}
	if err := syscall.Mount(btrfsRootfs, mountPoint, "bind", syscall.MS_BIND, ""); err != nil {
		return nil, fmt.Errorf("failed to mount %s to %s: %v", btrfsRootfs, mountPoint, err)
	}
	if readonly {
		if err := syscall.Mount(btrfsRootfs, mountPoint, "bind", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
			syscall.Unmount(mountPoint, syscall.MNT_DETACH)
			return nil, fmt.Errorf("failed to mount %s to %s readonly: %v", btrfsRootfs, mountPoint, err)
		}
	}

	containerPath := "/" + containerId
	vol := &runv.VolumeDescription{
		Name:     containerPath,
		Source:   containerPath,
		Fstype:   "dir",
		Format:   "vfs",
		ReadOnly: readonly,
	}

	return vol, nil
}

func (s *BtrfsStorage) CleanupContainer(id, sharedDir string) error {
	return syscall.Unmount(filepath.Join(sharedDir, id, "rootfs"), 0)
}

func (s *BtrfsStorage) InjectFile(src io.Reader, mountId, target, baseDir string, perm, uid, gid int) error {
	return storage.FsInjectFile(src, mountId, target, filepath.Dir(s.subvolumesDirID(mountId)), perm, uid, gid)
}

func (s *BtrfsStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	volName, err := storage.CreateVFSVolume(podId, spec.Name)
	if err != nil {
		return err
	}
	spec.Source = volName
	spec.Format = "vfs"
	spec.Fstype = "dir"
	return nil
}

func (s *BtrfsStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}

type RawBlockStorage struct {
	rootPath string
}

func RawBlockFactory(_ *dockertypes.Info, _ *daemondb.DaemonDB) (Storage, error) {
	driver := &RawBlockStorage{
		rootPath: filepath.Join(utils.HYPER_ROOT, "rawblock"),
	}
	return driver, nil
}

func (s *RawBlockStorage) Type() string {
	return "rawblock"
}

func (s *RawBlockStorage) RootPath() string {
	return s.rootPath
}

func (s *RawBlockStorage) Init(c *apitypes.HyperConfig) error {
	if err := os.MkdirAll(filepath.Join(s.RootPath(), "volumes"), 0700); err != nil {
		return err
	}
	return nil
}

func (*RawBlockStorage) CleanUp() error { return nil }

func (s *RawBlockStorage) PrepareContainer(containerId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
	devFullName := filepath.Join(s.RootPath(), "blocks", containerId)

	vol := &runv.VolumeDescription{
		Name:     devFullName,
		Source:   devFullName,
		Fstype:   "xfs",
		Format:   "raw",
		ReadOnly: readonly,
	}

	return vol, nil
}

func (s *RawBlockStorage) CleanupContainer(id, sharedDir string) error {
	return nil
}

func (s *RawBlockStorage) InjectFile(src io.Reader, mountId, target, baseDir string, perm, uid, gid int) error {
	if err := rawblock.GetImage(filepath.Join(s.RootPath(), "blocks"), baseDir, mountId, "xfs", "", uid, gid); err != nil {
		return err
	}
	defer rawblock.PutImage(baseDir, mountId)
	return storage.FsInjectFile(src, mountId, target, baseDir, perm, uid, gid)
}

func (s *RawBlockStorage) CreateVolume(podId string, spec *apitypes.UserVolume) error {
	block := filepath.Join(s.RootPath(), "volumes", fmt.Sprintf("%s-%s", podId, spec.Name))
	if err := rawblock.CreateBlock(block, "xfs", "", uint64(storage.DEFAULT_DM_VOL_SIZE)); err != nil {
		return err
	}
	spec.Source = block
	spec.Fstype = "xfs"
	spec.Format = "raw"
	return nil
}

func (s *RawBlockStorage) RemoveVolume(podId string, record []byte) error {
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

func (*VBoxStorage) Init(c *apitypes.HyperConfig) error { return nil }

func (*VBoxStorage) CleanUp() error { return nil }

func (v *VBoxStorage) PrepareContainer(mountId, sharedDir string, readonly bool) (*runv.VolumeDescription, error) {
	devFullName, err := vbox.MountContainerToSharedDir(mountId, v.RootPath(), "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}

	vol := &runv.VolumeDescription{
		Name:     devFullName,
		Source:   devFullName,
		Fstype:   "ext4",
		Format:   "vdi",
		ReadOnly: readonly,
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
