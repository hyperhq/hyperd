package overlay

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"hyper/utils"
)

func MountContainerToSharedDir(containerId, rootDir, sharedDir, mountLabel string) (string, error) {
	var (
		mountPoint = path.Join(sharedDir, containerId, "rootfs")
		upperDir   = path.Join(rootDir, containerId, "upper")
		workDir    = path.Join(rootDir, containerId, "work")
	)

	if _, err := os.Stat(mountPoint); err != nil {
		if err = os.MkdirAll(mountPoint, 0755); err != nil {
			return "", err
		}
	}
	lowerId, err := ioutil.ReadFile(path.Join(rootDir, containerId) + "/lower-id")
	if err != nil {
		return "", err
	}
	lowerDir := path.Join(rootDir, string(lowerId), "root")

	params := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	if err := syscall.Mount("overlay", mountPoint, "overlay", 0, utils.FormatMountLabel(params, mountLabel)); err != nil {
		return "", fmt.Errorf("error creating overlay mount to %s: %v", mountPoint, err)
	}
	return mountPoint, nil
}

func AttachFiles(containerId, fromFile, toDir, rootDir, perm, uid, gid string) error {
	if containerId == "" {
		return fmt.Errorf("Please make sure the arguments are not NULL!\n")
	}
	permInt, err := strconv.Atoi(perm)
	if err != nil {
		return err
	}
	// It just need the block device without copying any files
	// FIXME whether we need to return an error if the target directory is null
	if toDir == "" {
		return nil
	}
	// Make a new file with the given premission and wirte the source file content in it
	if _, err := os.Stat(fromFile); err != nil && os.IsNotExist(err) {
		return err
	}
	buf, err := ioutil.ReadFile(fromFile)
	if err != nil {
		return err
	}
	targetDir := path.Join(rootDir, containerId, "rootfs", toDir)
	_, err = os.Stat(targetDir)
	targetFile := targetDir
	if err != nil && os.IsNotExist(err) {
		// we need to create a target directory with given premission
		if err := os.MkdirAll(targetDir, os.FileMode(permInt)); err != nil {
			return err
		}
		targetFile = targetDir + "/" + filepath.Base(fromFile)
	} else {
		targetFile = targetDir + "/" + filepath.Base(fromFile)
	}
	err = ioutil.WriteFile(targetFile, buf, os.FileMode(permInt))
	if err != nil {
		return err
	}
	user_id, _ := strconv.Atoi(uid)
	err = syscall.Setuid(user_id)
	if err != nil {
		return err
	}
	group_id, _ := strconv.Atoi(gid)
	err = syscall.Setgid(group_id)
	if err != nil {
		return err
	}

	return nil
}
