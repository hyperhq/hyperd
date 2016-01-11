// +build !linux

package devicemapper

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// For device mapper, we do not need to mount the container to sharedDir.
// All of we need to provide the block device name of container.
func MountContainerToSharedDir(containerId, sharedDir, devPrefix string) (string, error) {
	return "", nil
}

func InjectFile(src io.Reader, containerId, devPrefix, target, rootPath string, perm, uid, gid int) error {
	return fmt.Errorf("Unsupported, inject file to %s is not supported in current arch", target)
}

func CreateNewDevice(containerId, devPrefix, rootPath string) error {
	return nil
}

func AttachFiles(containerId, devPrefix, fromFile, toDir, rootPath, perm, uid, gid string) error {
	return nil
}

func ProbeFsType(device string) (string, error) {
	// The daemon will only be run on Linux platform, so 'file -s' command
	// will be used to test the type of filesystem which the device located.
	cmd := fmt.Sprintf("file -sL %s", device)
	command := exec.Command("/bin/sh", "-c", cmd)
	fileCmdOutput, err := command.CombinedOutput()
	if err != nil {
		return "", nil
	}

	if strings.Contains(strings.ToLower(string(fileCmdOutput)), "ext") {
		return "ext4", nil
	}
	if strings.Contains(strings.ToLower(string(fileCmdOutput)), "xfs") {
		return "xfs", nil
	}

	return "", fmt.Errorf("Unknown filesystem type on %s", device)
}

type DeviceMapper struct {
	Datafile         string
	Metadatafile     string
	DataLoopFile     string
	MetadataLoopFile string
	Size             int
	PoolName         string
}

func CreatePool(dm *DeviceMapper) error {
	return nil
}

func CreateVolume(poolName, volName, dev_id string, size int, restore bool) error {
	return nil
}

func DeleteVolume(dm *DeviceMapper, dev_id int) error {
	return nil
}

func DMCleanup(dm *DeviceMapper) error {
	return nil
}
