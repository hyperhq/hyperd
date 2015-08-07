// +build linux

package utils

import (
	"syscall"
)

var (
	MS_BIND uintptr = syscall.MS_BIND
)

func Mount(source string, target string, fstype string, flags uintptr, data string) error {
	if err := syscall.Mount(source, target, fstype, flags, data); err != nil {
		return err
	}
	return nil
}
