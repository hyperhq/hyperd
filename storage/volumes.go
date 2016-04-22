package storage

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/utils"
)

func CreateVFSVolume(podId, shortName string) (string, error) {
	volName := path.Join("/var/tmp/hyper", podId, shortName)
	if _, err := os.Stat(volName); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(volName, os.FileMode(0777)); err != nil {
			return "", err
		}
	}
	return volName, nil
}

func MountVFSVolume(src, sharedDir string) (string, error) {
	var flags uintptr = utils.MS_BIND

	mountSharedDir := utils.RandStr(10, "alpha")
	targetDir := path.Join(sharedDir, mountSharedDir)
	glog.V(1).Infof("trying to bind dir %s to %s", src, targetDir)

	stat, err := os.Stat(src)
	if err != nil {
		glog.Error("Cannot stat volume Source ", err.Error())
		return "", err
	}

	if runtime.GOOS == "linux" {
		base := filepath.Dir(targetDir)
		if err := os.MkdirAll(base, 0755); err != nil && !os.IsExist(err) {
			glog.Errorf("error to create dir %s for volume %s", base, src)
			return "", err
		}

		if stat.IsDir() {
			if err := os.MkdirAll(targetDir, 0755); err != nil && !os.IsExist(err) {
				glog.Errorf("error to create dir %s for volume %s", targetDir, src)
				return "", err
			}
		} else if f, err := os.Create(targetDir); err != nil && !os.IsExist(err) {
			glog.Errorf("error to create file %s for volume %s", targetDir, src)
			return "", err
		} else if err == nil {
			f.Close()
		}
	}

	if err := utils.Mount(src, targetDir, "none", flags, "--bind"); err != nil {
		glog.Errorf("bind dir %s failed: %s", src, err.Error())
		return "", err
	}

	return mountSharedDir, nil
}

func UmountVFSVolume(vol, sharedDir string) error {
	mount := path.Join(sharedDir, vol)

	err := syscall.Unmount(mount, 0)
	if err != nil {
		glog.Warningf("Cannot umount volume %s: %s", mount, err.Error())
		err = syscall.Unmount(mount, syscall.MNT_DETACH)
		if err != nil {
			glog.Warningf("Cannot lazy umount volume %s: %s", mount, err.Error())
			return err
		}
	}

	os.Remove(mount)
	return nil
}
