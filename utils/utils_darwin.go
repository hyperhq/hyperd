// +build darwin

package utils

import (
	"syscall"
)

var (
	MS_BIND uintptr = 0
)

func Mount(source string, target string, fstype string, flags uintptr, data string) error {
	// bind mount is treated as hard link
	if err := syscall.Link(source, target); err != nil {
		return err
	}
	return nil
}
