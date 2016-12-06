package pod

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/storage"
	dm "github.com/hyperhq/hyperd/storage/devicemapper"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
)

func GetMountIdByContainer(driver, cid string) (string, error) {
	idPath := path.Join(utils.HYPER_ROOT, fmt.Sprintf("image/%s/layerdb/mounts/%s/mount-id", driver, cid))
	if _, err := os.Stat(idPath); err != nil && os.IsNotExist(err) {
		return "", err
	}

	id, err := ioutil.ReadFile(idPath)
	if err != nil {
		return "", err
	}

	return string(id), nil
}

func ProbeExistingVolume(v *apitypes.UserVolume, sharedDir string) (*runv.VolumeDescription, error) {
	if v == nil || v.Source == "" { //do not create volume in this function, it depends on storage driver.
		return nil, fmt.Errorf("can not generate volume info from %v", v)
	}

	var err error = nil
	vol := &runv.VolumeDescription{
		Name:   v.Name,
		Source: v.Source,
		Format: v.Format,
		Fstype: v.Fstype,
	}

	if v.Option != nil {
		vol.Options = &runv.VolumeOption{
			User:     v.Option.User,
			Monitors: v.Option.Monitors,
			Keyring:  v.Option.Keyring,
		}
	}

	if v.Format == "vfs" {
		vol.Fstype = "dir"
		vol.Source, err = storage.MountVFSVolume(v.Source, sharedDir)
		if err != nil {
			return nil, err
		}
		hlog.Log(DEBUG, "dir %s is bound to %s", v.Source, vol.Source)
	} else if v.Format == "raw" && v.Fstype == "" {
		vol.Fstype, err = dm.ProbeFsType(v.Source)
		if err != nil {
			vol.Fstype = storage.DEFAULT_VOL_FS
			err = nil
		}
	}

	return vol, nil
}

func UmountExistingVolume(fstype, target, sharedDir string) error {
	if fstype == "dir" {
		return storage.UmountVFSVolume(target, sharedDir)
	}
	if !path.IsAbs(target) {
		return nil
	}
	return dm.UnmapVolume(target)
}
