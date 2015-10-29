package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"syscall"
)

func FsInjectFile(src io.Reader, containerId, target, rootDir string, perm, uid, gid int) error {
	if containerId == "" {
		return fmt.Errorf("Please make sure the arguments are not NULL!\n")
	}

	targetFile := path.Join(rootDir, containerId, "rootfs", target)

	return WriteFile(src, targetFile, perm, uid, gid)

}

func WriteFile(src io.Reader, targetFile string, permFile, uid, gid int) error {

	targetDir := filepath.Dir(targetFile)
	permDir := permFile | 0111

	if stat, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(targetDir, os.FileMode(permDir))
		}
		if err != nil {
			return err
		}
	} else if !stat.IsDir() {
		return errors.New("File target is not a dir: " + targetDir)
	}

	f, err := os.OpenFile(targetFile, os.O_RDWR|os.O_CREATE, os.FileMode(permFile))
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, src)
	if err != nil {
		return err
	}

	if err = syscall.Chown(targetFile, uid, gid); err != nil {
		return err
	}

	return nil

}
