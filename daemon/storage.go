package daemon

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	dockertypes "github.com/hyperhq/hyper/lib/docker/api/types"
	"github.com/hyperhq/hyper/storage"
	"github.com/hyperhq/hyper/storage/aufs"
	dm "github.com/hyperhq/hyper/storage/devicemapper"
	"github.com/hyperhq/hyper/storage/overlay"
	"github.com/hyperhq/hyper/storage/vbox"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/glog"
)

const (
	DEFAULT_DM_POOL      string = "hyper-volume-pool"
	DEFAULT_DM_POOL_SIZE int    = 20971520 * 512
	DEFAULT_DM_DATA_LOOP string = "/dev/loop6"
	DEFAULT_DM_META_LOOP string = "/dev/loop7"
	DEFAULT_DM_VOL_SIZE  int    = 2 * 1024 * 1024 * 1024
)

type Storage interface {
	Type() string
	RootPath() string

	Init() error
	CleanUp() error

	PrepareContainer(id, sharedir string) (*hypervisor.ContainerInfo, error)
	InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error
	CreateVolume(daemon *Daemon, podId, shortName string) (*hypervisor.VolumeInfo, error)
	RemoveVolume(podId string, record []byte) error
}

var StorageDrivers map[string]func(*dockertypes.Info) (Storage, error) = map[string]func(*dockertypes.Info) (Storage, error){
	"devicemapper": DMFactory,
	"aufs":         AufsFactory,
	"overlay":      OverlayFsFactory,
	"vbox":         VBoxStorageFactory,
}

func StorageFactory(sysinfo *dockertypes.Info) (Storage, error) {
	if factory, ok := StorageDrivers[sysinfo.Driver]; ok {
		return factory(sysinfo)
	}
	return nil, fmt.Errorf("hyperd can not support docker's backing storage: %s", sysinfo.Driver)
}

type DevMapperStorage struct {
	CtnPoolName   string
	VolPoolName string
	DevPrefix  string
	FsType     string
	rootPath   string
	DmPoolData *dm.DeviceMapper
}

func DMFactory(sysinfo *dockertypes.Info) (Storage, error) {
	driver := &DevMapperStorage{}

	driver.VolPoolName = DEFAULT_DM_POOL

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
	driver.rootPath = path.Join(utils.HYPER_ROOT, "devicemapper")
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
		Datafile:         path.Join(utils.HYPER_ROOT, "lib") + "/data",
		Metadatafile:     path.Join(utils.HYPER_ROOT, "lib") + "/metadata",
		DataLoopFile:     DEFAULT_DM_DATA_LOOP,
		MetadataLoopFile: DEFAULT_DM_META_LOOP,
		PoolName:         dms.VolPoolName,
		Size:             DEFAULT_DM_POOL_SIZE,
	}
	dms.DmPoolData = &dmPool

	// Prepare the DeviceMapper storage
	return dm.CreatePool(&dmPool)
}

func (dms *DevMapperStorage) CleanUp() error {
	return dm.DMCleanup(dms.DmPoolData)
}

func (dms *DevMapperStorage) PrepareContainer(id, sharedDir string) (*hypervisor.ContainerInfo, error) {
	if err := dm.CreateNewDevice(id, dms.DevPrefix, dms.RootPath()); err != nil {
		return nil, err
	}
	devFullName, err := dm.MountContainerToSharedDir(id, sharedDir, dms.DevPrefix)
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}
	fstype, err := dm.ProbeFsType(devFullName)
	if err != nil {
		fstype = "ext4"
	}
	return &hypervisor.ContainerInfo{
		Id:     id,
		Rootfs: "/rootfs",
		Image:  devFullName,
		Fstype: fstype,
	}, nil
}

func (dms *DevMapperStorage) InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	return dm.InjectFile(src, containerId, dms.DevPrefix, target, rootDir, perm, uid, gid)
}

func (dms *DevMapperStorage) CreateVolume(daemon *Daemon, podId, shortName string) (*hypervisor.VolumeInfo, error) {
	volName := fmt.Sprintf("%s-%s-%s", dms.VolPoolName, podId, shortName)
	dev_id, _ := daemon.GetVolumeId(podId, volName)
	glog.Infof("DeviceID is %d", dev_id)

	restore := true
	if dev_id < 1 {
		dev_id, _ = daemon.GetMaxDeviceId()
		dev_id++
		restore = false
	}
	dev_id_str := strconv.Itoa(dev_id)
	err := dm.CreateVolume(dms.VolPoolName, volName, dev_id_str, DEFAULT_DM_VOL_SIZE, restore)
	if err != nil {
		return nil, err
	}
	daemon.SetVolumeId(podId, volName, dev_id_str)

	fstype, err := dm.ProbeFsType("/dev/mapper/" + volName)
	if err != nil {
		fstype = "ext4"
	}

	glog.V(1).Infof("volume %s created with dm as %s", shortName, volName)

	return &hypervisor.VolumeInfo{
		Name:     shortName,
		Filepath: path.Join("/dev/mapper/", volName),
		Fstype:   fstype,
		Format:   "raw",
	}, nil
}

func (dms *DevMapperStorage) RemoveVolume(podId string, record []byte) error {
	fields := strings.Split(string(record), ":")
	dev_id, _ := strconv.Atoi(fields[1])
	if err := dm.DeleteVolume(dms.DmPoolData, dev_id); err != nil {
		glog.Error(err.Error())
		return err
	}
	return nil
}

type AufsStorage struct {
	rootPath string
}

func AufsFactory(sysinfo *dockertypes.Info) (Storage, error) {
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

func (a *AufsStorage) PrepareContainer(id, sharedDir string) (*hypervisor.ContainerInfo, error) {
	_, err := aufs.MountContainerToSharedDir(id, a.RootPath(), sharedDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}
	devFullName := "/" + id + "/rootfs"
	return &hypervisor.ContainerInfo{
		Id:     id,
		Rootfs: "",
		Image:  devFullName,
		Fstype: "dir",
	}, nil
}

func (a *AufsStorage) InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	return storage.FsInjectFile(src, containerId, target, rootDir, perm, uid, gid)
}

func (a *AufsStorage) CreateVolume(daemon *Daemon, podId, shortName string) (*hypervisor.VolumeInfo, error) {
	volName, err := storage.CreateVFSVolume(podId, shortName)
	if err != nil {
		return nil, err
	}
	return &hypervisor.VolumeInfo{
		Name:     shortName,
		Filepath: volName,
		Fstype:   "dir",
	}, nil
}

func (a *AufsStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}

type OverlayFsStorage struct {
	rootPath string
}

func OverlayFsFactory(_ *dockertypes.Info) (Storage, error) {
	driver := &OverlayFsStorage{
		rootPath: path.Join(utils.HYPER_ROOT, "overlay"),
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

func (o *OverlayFsStorage) PrepareContainer(id, sharedDir string) (*hypervisor.ContainerInfo, error) {
	_, err := overlay.MountContainerToSharedDir(id, o.RootPath(), sharedDir, "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}
	devFullName := "/" + id + "/rootfs"
	return &hypervisor.ContainerInfo{
		Id:     id,
		Rootfs: "",
		Image:  devFullName,
		Fstype: "dir",
	}, nil
}

func (o *OverlayFsStorage) InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	return storage.FsInjectFile(src, containerId, target, rootDir, perm, uid, gid)
}

func (o *OverlayFsStorage) CreateVolume(daemon *Daemon, podId, shortName string) (*hypervisor.VolumeInfo, error) {
	volName, err := storage.CreateVFSVolume(podId, shortName)
	if err != nil {
		return nil, err
	}
	return &hypervisor.VolumeInfo{
		Name:     shortName,
		Filepath: volName,
		Fstype:   "dir",
	}, nil
}

func (o *OverlayFsStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}

type VBoxStorage struct {
	rootPath string
}

func VBoxStorageFactory(_ *dockertypes.Info) (Storage, error) {
	driver := &VBoxStorage{
		rootPath: path.Join(utils.HYPER_ROOT, "vbox"),
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

func (v *VBoxStorage) PrepareContainer(id, sharedDir string) (*hypervisor.ContainerInfo, error) {
	devFullName, err := vbox.MountContainerToSharedDir(id, v.RootPath(), "")
	if err != nil {
		glog.Error("got error when mount container to share dir ", err.Error())
		return nil, err
	}
	return &hypervisor.ContainerInfo{
		Id:     id,
		Rootfs: "/rootfs",
		Image:  devFullName,
		Fstype: "ext4",
	}, nil
}

func (v *VBoxStorage) InjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	return errors.New("vbox storage driver does not support file insert yet")
}

func (v *VBoxStorage) CreateVolume(daemon *Daemon, podId, shortName string) (*hypervisor.VolumeInfo, error) {
	volName, err := storage.CreateVFSVolume(podId, shortName)
	if err != nil {
		return nil, err
	}
	return &hypervisor.VolumeInfo{
		Name:     shortName,
		Filepath: volName,
		Fstype:   "dir",
	}, nil
}

func (v *VBoxStorage) RemoveVolume(podId string, record []byte) error {
	return nil
}
