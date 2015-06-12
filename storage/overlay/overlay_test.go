package overlay

import (
	"bufio"
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
	overlaySupport = false
	temp           = os.TempDir()
	tempDir        = path.Join(temp, "overlay-test")
	sharedTempDir  = path.Join(tempDir, "shared")
	containerId    = "123"
	lowerId        = "12345"
	lowerDir       = path.Join(tempDir, lowerId)
	containerDir   = path.Join(tempDir, containerId)
	workDir        = path.Join(containerDir, "work")
	upperDir       = path.Join(containerDir, "upper")
	loweridFile    = fmt.Sprintf("%s/lower-id", containerDir)
	testFile3      = fmt.Sprintf("%s/root/3", lowerDir)
	testFile2      = fmt.Sprintf("%s/root/2", lowerDir)
	testFile1      = fmt.Sprintf("%s/root/1", lowerDir)
)

func init() {
	overlaySupport = supportOverlay()
}

func InitDir() error {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(lowerDir, "root"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		return err
	}

	return nil
}

func Cleanup() error {
	if err := os.RemoveAll(tempDir); err != nil {
		return err
	}

	return nil
}

func InitFile() error {
	file1Data := ""
	file2Data := "1\n"
	file3Data := "1\n2\n"

	if err := ioutil.WriteFile(testFile1, []byte(file1Data), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(testFile2, []byte(file2Data), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(testFile3, []byte(file3Data), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(loweridFile, []byte(lowerId), 0755); err != nil {
		return err
	}

	return nil
}

func supportOverlay() bool {
	// We can try to modprobe overlay first before looking at
	// proc/filesystems for when overlay is supported
	if os.Getgid() == 0 {
		if err := exec.Command("/bin/sh", "-c", "modprobe", "overlay").Run(); err != nil {
			exec.Command("/bin/sh", "-c", "modprobe", "overlayfs").Run()
		}
	}

	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return false
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "overlay") {
			return true
		}
	}
	return false
}

func TestTempDirCreate(t *testing.T) {
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}

}

func TestMountContainerToShareDir(t *testing.T) {
	if os.Getgid() != 0 {
		t.Errorf("This test case should be run in root group")
		return
	}
	if overlaySupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(tempDir); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Temp dir create failed")
		} else {
			t.Fatalf("Temp dir create failed, %s", err.Error())
		}
	}

	if err := InitFile(); err != nil {
		t.Fatalf("Error during creating the test file: %s\n", err.Error())
	}
	if _, err := os.Stat(testFile3); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("lower files create failed")
		} else {
			t.Fatalf("lower files create failed, %s", err.Error())
		}
	}
	if err := os.MkdirAll(sharedTempDir, 0755); err != nil {
		t.Fatalf("Shared dir create failed, %s", err.Error())
	}
	if _, err := os.Stat(sharedTempDir); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("shared dir create failed")
		} else {
			t.Fatalf("shared dir create failed, %s", err.Error())
		}
	}

	t.Log("Mount the parent read-only images and container")
	mountPoint, err := MountContainerToSharedDir(containerId, tempDir, sharedTempDir, "")
	if err != nil {
		t.Fatalf("Error during mounting paths: %s\n", err.Error())
	}
	t.Logf(mountPoint)

	// verify the mount result
	t.Log("Verify the mount result")
	if _, err := os.Stat(path.Join(mountPoint, "1")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (1) does not mount successfully!")
	}
	if _, err := os.Stat(path.Join(mountPoint, "2")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (2) does not mount successfully!")
	}
	if _, err := os.Stat(path.Join(mountPoint, "3")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (3) does not mount successfully!")
	}

	if err := os.MkdirAll(path.Join(mountPoint, "haha"), 0755); err != nil {
		t.Fatalf("Error on mkdir in mounted target dir!")
	}
	if _, err := os.Stat(path.Join(mountPoint, "haha")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The dir in mounted target dir does not exist!")
	}

	// Unmount the target dir and then remove it
	if err := syscall.Unmount(mountPoint, 0); err != nil {
		t.Fatalf("Unmount error: %s", err.Error())
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}

func TestAttachFiles(t *testing.T) {
	if os.Getgid() != 0 {
		t.Errorf("This test case should be run in root group")
		return
	}
	if overlaySupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(tempDir); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Temp dir create failed")
		} else {
			t.Fatalf("Temp dir create failed, %s", err.Error())
		}
	}

	if err := InitFile(); err != nil {
		t.Fatalf("Error during creating the test file: %s\n", err.Error())
	}
	if _, err := os.Stat(testFile3); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("layer files create failed")
		} else {
			t.Fatalf("layer files create failed, %s", err.Error())
		}
	}
	if err := os.MkdirAll(sharedTempDir, 0755); err != nil {
		t.Fatalf("Shared dir create failed, %s", err.Error())
	}
	if _, err := os.Stat(sharedTempDir); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("shared dir create failed")
		} else {
			t.Fatalf("shared dir create failed, %s", err.Error())
		}
	}

	t.Log("MountContainerToSharedDir do")
	mountPoint, err := MountContainerToSharedDir(containerId, tempDir, sharedTempDir, "")
	if err != nil {
		t.Fatalf("Error during mounting paths: %s\n", err.Error())
	}

	t.Log("AttachFiles do")
	err = AttachFiles(containerId, "/etc/os-release", "/", sharedTempDir, "0755", "0", "0")
	if err != nil {
		t.Fatalf(err.Error())
	}

	t.Log("Verify the AttachFiles function execution result")
	if _, err := os.Stat(mountPoint + "/os-release"); err != nil && os.IsNotExist(err) {
		t.Fatalf("The dir in mounted target dir does not exist!")
	}

	// Unmount the target dir and then remove it
	if err := syscall.Unmount(mountPoint, 0); err != nil {
		t.Fatalf("Unmount error: %s", err.Error())
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}
