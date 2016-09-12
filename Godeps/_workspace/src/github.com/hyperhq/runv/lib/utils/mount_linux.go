package utils

import (
	"os"
	"syscall"
)

func Mount(src, dst string, readOnly bool) error {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		os.MkdirAll(dst, 0755)
	}

	err := syscall.Mount(src, dst, "", syscall.MS_BIND|syscall.MS_REC, "")
	if err == nil && readOnly {
		err = syscall.Mount(src, dst, "", syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_REMOUNT|syscall.MS_REC, "")
	}
	return err
}

func Umount(root string) {
	syscall.Unmount(root, syscall.MNT_DETACH)
}
