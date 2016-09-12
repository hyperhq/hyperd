package utils

import (
	"syscall"
)

func Mount(src, dst string, readOnly bool) error {
	//return syscall.Symlink(src, dst)
	return syscall.Link(src, dst)
}

func Umount(path string) {
	syscall.Unlink(path)
}
