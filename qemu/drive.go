package qemu

import (
	"fmt"
	"os/exec"
	"path"
	"syscall"
	"time"

	"hyper/lib/glog"
	"hyper/pod"
	"hyper/storage/aufs"
)

func CreateContainer(userPod *pod.UserPod, sharedDir string, hub chan QemuEvent) (string, error) {
	return "", nil
}

func UmountOverlayContainer(shareDir, image string, index int, hub chan QemuEvent) {
	mount := path.Join(shareDir, image)
	success := true
	for i := 0; i < 10; i++ {
		time.Sleep(3 * time.Second / 1000)
		err := syscall.Unmount(mount, 0)
		if err != nil {
			glog.Warningf("Cannot umount overlay %s: %s", mount, err.Error())
			success = false
		} else {
			success = true
			break
		}
	}
	hub <- &ContainerUnmounted{Index: index, Success: success}
}

func UmountAufsContainer(shareDir, image string, index int, hub chan QemuEvent) {
	mount := path.Join(shareDir, image)
	success := true
	for i := 0; i < 10; i++ {
		time.Sleep(3 * time.Second / 1000)
		err := aufs.Unmount(mount)
		if err != nil {
			glog.Warningf("Cannot umount aufs %s: %s", mount, err.Error())
			success = false
		} else {
			success = true
			break
		}
	}
	hub <- &ContainerUnmounted{Index: index, Success: success}
}

func UmountVolume(shareDir, volPath string, name string, hub chan QemuEvent) {
	mount := path.Join(shareDir, volPath)
	success := true
	err := syscall.Unmount(mount, 0)
	if err != nil {
		glog.Warningf("Cannot umount volume %s: %s", mount, err.Error())
		success = false
	}
	// After umount that device, we need to delete it
	hub <- &VolumeUnmounted{Name: name, Success: success}
}

func UmountDMDevice(deviceFullPath, name string, hub chan QemuEvent) {
	args := fmt.Sprintf("dmsetup remove -f %s", deviceFullPath)
	cmd := exec.Command("/bin/sh", "-c", args)
	success := true
	if output, err := cmd.CombinedOutput(); err != nil {
		glog.Warningf("Cannot umount device %s: %s, %s", deviceFullPath, err.Error(), output)
		// retry
		cmd := exec.Command("/bin/sh", "-c", args)
		if err := cmd.Run(); err != nil {
			success = false
		}
	} else {
		// Command was successful
		success = true
	}

	// After umount that device, we need to delete it
	hub <- &BlockdevRemovedEvent{Name: name, Success: success}
}
