package devicemapper

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
)

var (
	poolName         = "dm-1:1-1234567-pool"
	tempDir          = os.TempDir()
	dmRootDir        = path.Join(tempDir, "devicemapper")
	datafile         = dmRootDir + "/data"
	metadatafile     = dmRootDir + "/meta_data"
	dataLoopFile     = "/dev/loop7"
	metadataLoopFile = "/dev/loop6"
	containerId      = "123"
	devPrefix        = poolName[:strings.Index(poolName, "-pool")]
)

// Do some init work, such as creating a dm pool
func Init() error {
	// Create the root dir for devicemapper
	if err := os.MkdirAll(dmRootDir, 0755); err != nil {
		return err
	}

	// Create data file and metadata file
	parms := fmt.Sprintf("dd if=/dev/zero of=%s bs=1G count=1", datafile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("dd if=/dev/zero of=%s bs=128M count=1", metadatafile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}

	// Setup the loop device for data and metadata files
	parms = fmt.Sprintf("losetup %s %s", dataLoopFile, datafile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("losetup %s %s", metadataLoopFile, metadatafile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}

	// Make filesystem for data loop device and metadata loop device
	parms = fmt.Sprintf("mkfs.ext4 %s", dataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("mkfs.ext4 %s", metadataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}

	// Zero the loop device
	parms = fmt.Sprintf("dd if=/dev/zero of=%s bs=1G count=1", dataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("dd if=/dev/zero of=%s bs=128M count=1", metadataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}

	// Create the thin pool for test
	parms = fmt.Sprintf("dmsetup create %s --table '0 2097152 thin-pool %s %s 128 0'", poolName, metadataLoopFile, dataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("dmsetup message /dev/mapper/%s 0 'create_thin' 0", poolName)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}

	// Write the container's device metadata
	if err := os.MkdirAll(path.Join(dmRootDir, "metadata"), 0755); err != nil {
		return err
	}
	// Write the container's device mnt
	if err := os.MkdirAll(path.Join(dmRootDir, "mnt"), 0755); err != nil {
		return err
	}

	jsondata := jsonMetadata{
		Device_id:      0,
		Size:           262144,
		Transaction_id: 2,
		Initialized:    false,
	}
	body, err := json.Marshal(jsondata)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(dmRootDir+"/metadata/"+containerId, body, 0755); err != nil {
		return err
	}
	return nil
}

// Delete the pool which is created in 'Init' function
func Cleanup() error {
	var parms string
	parms = fmt.Sprintf("dmsetup remove -f \"/dev/mapper/%s-%s\"", devPrefix, containerId)
	if output, err := exec.Command("/bin/sh", "-c", parms).Output(); err != nil {
		//fmt.Printf("Exec (%s) failed: %s\n", parms, string(output))
		_ = output
		return err
	}

	// Delete the thin pool for test
	parms = fmt.Sprintf("dmsetup remove \"/dev/mapper/%s\"", poolName)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	// Delete the loop device
	parms = fmt.Sprintf("losetup -d %s", metadataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	parms = fmt.Sprintf("losetup -d %s", dataLoopFile)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		return err
	}
	if err := os.RemoveAll(dmRootDir); err != nil {
		return err
	}
	return nil
}

func TestCreateNewDevice(t *testing.T) {
	Cleanup()
	if err := Init(); err != nil {
		t.Fatalf(err.Error())
	}

	if err := CreateNewDevice(containerId, devPrefix, dmRootDir); err != nil {
		t.Fatalf(err.Error())
	}

	if _, err := os.Stat("/dev/mapper/" + devPrefix + "-" + containerId); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Create device for container failed")
		} else {
			t.Fatalf("Create device for container failed, %s", err.Error())
		}
	}

	if err := Cleanup(); err != nil {
		t.Fatalf(err.Error())
	}
}

func TestProbeFsType(t *testing.T) {
	Cleanup()
	if err := Init(); err != nil {
		t.Fatalf(err.Error())
	}

	if err := CreateNewDevice(containerId, devPrefix, dmRootDir); err != nil {
		t.Fatalf(err.Error())
	}

	devFullName := "/dev/mapper/" + devPrefix + "-" + containerId
	var parms string
	parms = fmt.Sprintf("mkfs.ext4 \"%s\"", devFullName)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		t.Fatalf(err.Error())
	}
	fstype, err := ProbeFsType("/dev/dm-1")
	if err != nil {
		t.Fatalf(err.Error())
	}

	if fstype == "" {
		t.Error("Failed to check the device filesystem type!")
	}

	defer func() {
		if err := Cleanup(); err != nil {
			t.Fatalf(err.Error())
		}
	}()
}

func TestAttachFiles(t *testing.T) {
	Cleanup()
	if err := Init(); err != nil {
		t.Fatalf(err.Error())
	}
	if err := CreateNewDevice(containerId, devPrefix, dmRootDir); err != nil {
		t.Fatalf(err.Error())
	}
	parms := fmt.Sprintf("mkfs.ext4 \"/dev/mapper/%s-%s\"", devPrefix, containerId)
	if err := exec.Command("/bin/sh", "-c", parms).Run(); err != nil {
		t.Fatalf(err.Error())
	}

	var (
		flags       uintptr = syscall.MS_MGC_VAL
		idMountPath         = path.Join(dmRootDir, "mnt", containerId)
		rootFs              = path.Join(idMountPath, "rootfs")
		toDir               = "/"
		targetDir           = path.Join(rootFs, toDir)
		devFullName         = "/dev/mapper/" + devPrefix + "-" + containerId
	)
	if err := AttachFiles(containerId, devPrefix, "/etc/os-release", toDir, dmRootDir, "0755", "0", "0"); err != nil {
		t.Fatalf(err.Error())
	}

	fstype, err := ProbeFsType("/dev/mapper/dm-1:1-1234567-123")
	if err != nil {
		t.Fatalf(err.Error())
	}
	options := ""
	if fstype == "xfs" {
		// XFS needs nouuid or it can't mount filesystems with the same fs
		options = joinMountOptions(options, "nouuid")
	}

	err = syscall.Mount(devFullName, idMountPath, fstype, flags, joinMountOptions("discard", options))
	if err != nil && err == syscall.EINVAL {
		err = syscall.Mount(devFullName, idMountPath, fstype, flags, options)
	}
	if err != nil {
		t.Fatalf("Error mounting '%s' on '%s': %s", devFullName, idMountPath, err)
	}
	targetFile := targetDir + "/os-release"
	if _, err := os.Stat(targetFile); err != nil {
		t.Log(targetFile)
		t.Fatalf(err.Error())
	}
	syscall.Unmount(idMountPath, syscall.MNT_DETACH)
	defer func() {
		if err := Cleanup(); err != nil {
			t.Fatalf(err.Error())
		}
	}()
}
