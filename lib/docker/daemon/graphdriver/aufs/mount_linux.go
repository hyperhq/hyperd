package aufs

import (
	"os/exec"
	"syscall"

	"github.com/golang/glog"
)

const MsRemount = syscall.MS_REMOUNT

func mount(source string, target string, fstype string, flags uintptr, data string) error {
	return syscall.Mount(source, target, fstype, flags, data)
}

func Unmount(target string) error {
	if err := exec.Command("auplink", target, "flush").Run(); err != nil {
		glog.Errorf("Couldn't run auplink before unmount: %s", err)
	}
	if err := syscall.Unmount(target, 0); err != nil {
		return err
	}
	return nil
}
