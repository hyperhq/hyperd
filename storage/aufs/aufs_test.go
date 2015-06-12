package aufs

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

var (
	aufsSupport   = false
	temp          = os.TempDir()
	aufsTempDir   = path.Join(temp, "aufs-test")
	sharedTempDir = path.Join(aufsTempDir, "shared")
	layersTempDir = path.Join(aufsTempDir, "layers")
	diffTempDir   = path.Join(aufsTempDir, "diff")
	testFile3     = fmt.Sprintf("%s/3", layersTempDir)
	testFile2     = fmt.Sprintf("%s/2", layersTempDir)
	testFile1     = fmt.Sprintf("%s/1", layersTempDir)
)

func init() {
	aufsSupport = supportAufs()
}

func InitDir() error {
	if err := os.MkdirAll(aufsTempDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(layersTempDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(diffTempDir, 0755); err != nil {
		return err
	}
	return nil
}

func Cleanup() error {
	if err := os.RemoveAll(aufsTempDir); err != nil {
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

	return nil
}

func InitDiffFile() error {
	file1Data := "3"
	file2Data := "2"
	file3Data := "1"

	if err := os.MkdirAll(path.Join(diffTempDir, "1"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(diffTempDir, "2"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(diffTempDir, "3"), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(diffTempDir, "1")+"/1", []byte(file1Data), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(diffTempDir, "2")+"/2", []byte(file2Data), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(diffTempDir, "3")+"/3", []byte(file3Data), 0755); err != nil {
		return err
	}

	return nil
}

func supportAufs() bool {
	// We can try to modprobe aufs first before looking at
	// proc/filesystems for when aufs is supported
	if os.Getgid() == 0 {
		exec.Command("/bin/sh", "-c", "modprobe", "aufs").Run()
	}

	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return false
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "aufs") {
			return true
		}
	}
	return false
}

func TestTempDirCreate(t *testing.T) {
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(aufsTempDir); err != nil {
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
			t.Fatalf("Temp dir create failed")
		} else {
			t.Fatalf("Temp dir create failed, %s", err.Error())
		}
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}

}

func TestGetParentIds(t *testing.T) {
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}

	if err := InitFile(); err != nil {
		t.Fatalf("Error during creating the test file: %s\n", err.Error())
	}

	parentIds, err := getParentIds(path.Join(layersTempDir, "3"))
	if err != nil {
		t.Fatalf("%s\n", err.Error())
	}
	if !(parentIds[0] == "1" && parentIds[1] == "2") {
		t.Fatalf("Error to find the parent IDs\n")
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}

func TestGetParentDiffPaths(t *testing.T) {
	if aufsSupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}

	if err := InitFile(); err != nil {
		t.Fatalf("Error during creating the test file: %s\n", err.Error())
	}

	parentIdPaths, err := getParentDiffPaths("3", aufsTempDir)
	if err != nil {
		t.Fatalf("%s\n", err.Error())
	}
	if !(strings.Contains(parentIdPaths[0], fmt.Sprintf("%s/1", diffTempDir))) {
		t.Fatalf("Error to find the parent ID path\n")
	}
	if !(strings.Contains(parentIdPaths[1], fmt.Sprintf("%s/2", diffTempDir))) {
		t.Fatalf("Error to find the parent ID path\n")
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}

func TestAufsMount(t *testing.T) {
	if os.Getgid() != 0 {
		t.Errorf("This test case should be run in root group")
		return
	}
	t.Logf("aufs temp dir is %s", aufsTempDir)
	if aufsSupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(aufsTempDir); err != nil {
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
	if err := InitDiffFile(); err != nil {
		t.Fatalf("Error during creating the diff data file: %s\n", err.Error())
	}
	if _, err := os.Stat(path.Join(diffTempDir, "3")); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("diff files create failed")
		} else {
			t.Fatalf("diff files create failed, %s", err.Error())
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

	diffs, err := getParentDiffPaths("3", aufsTempDir)
	if err != nil {
		t.Fatalf("Error during getting parent diff paths: %s\n", err.Error())
	}

	t.Log("Mount the parent read-only images and container")
	if err := aufsMount(diffs, path.Join(diffTempDir, "3"), sharedTempDir, ""); err != nil {
		t.Fatalf("Error during mounting paths: %s\n", err.Error())
	}

	// verify the mount result
	t.Log("Verify the mount result")
	if _, err := os.Stat(path.Join(sharedTempDir, "1")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (1) does not mount successfully!")
	}
	if _, err := os.Stat(path.Join(sharedTempDir, "2")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (2) does not mount successfully!")
	}
	if _, err := os.Stat(path.Join(sharedTempDir, "3")); err != nil && os.IsNotExist(err) {
		t.Fatalf("The file (3) does not mount successfully!")
	}

	if err := os.MkdirAll(path.Join(sharedTempDir, "haha"), 0755); err != nil {
		t.Fatalf("Error on mkdir in mounted target dir!")
	}
	if _, err := os.Stat(path.Join(diffTempDir, "3") + "/haha"); err != nil && os.IsNotExist(err) {
		t.Fatalf("The dir in mounted target dir does not exist!")
	}

	// Unmount the target dir and then remove it
	Unmount(sharedTempDir)
	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}

func TestMountContainerToShareDir(t *testing.T) {
	if os.Getgid() != 0 {
		t.Errorf("This test case should be run in root group")
		return
	}
	if aufsSupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(aufsTempDir); err != nil {
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
	if err := InitDiffFile(); err != nil {
		t.Fatalf("Error during creating the diff data file: %s\n", err.Error())
	}
	if _, err := os.Stat(path.Join(diffTempDir, "3")); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("diff files create failed")
		} else {
			t.Fatalf("diff files create failed, %s", err.Error())
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
	mountPoint, err := MountContainerToSharedDir("3", aufsTempDir, sharedTempDir, "")
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
	if err := Unmount(mountPoint); err != nil {
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
	if aufsSupport == false {
		return
	}
	if err := InitDir(); err != nil {
		t.Fatalf("Error during creating the temp directory: %s\n", err.Error())
	}
	if _, err := os.Stat(aufsTempDir); err != nil {
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
	if err := InitDiffFile(); err != nil {
		t.Fatalf("Error during creating the diff data file: %s\n", err.Error())
	}
	if _, err := os.Stat(path.Join(diffTempDir, "3")); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("diff files create failed")
		} else {
			t.Fatalf("diff files create failed, %s", err.Error())
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
	mountPoint, err := MountContainerToSharedDir("3", aufsTempDir, sharedTempDir, "")
	if err != nil {
		t.Fatalf("Error during mounting paths: %s\n", err.Error())
	}

	t.Log("AttachFiles do")
	err = AttachFiles("3", "/etc/os-release", "/", sharedTempDir, "0755", "0", "0")
	if err != nil {
		t.Fatalf(err.Error())
	}

	t.Log("Verify the AttachFiles function execution result")
	if _, err := os.Stat(mountPoint + "/os-release"); err != nil && os.IsNotExist(err) {
		t.Fatalf("The dir in mounted target dir does not exist!")
	}

	// Unmount the target dir and then remove it
	if err := Unmount(mountPoint); err != nil {
		t.Fatalf("Unmount error: %s", err.Error())
	}

	if err := Cleanup(); err != nil {
		t.Fatalf("Error during removing files and dirs: %s\n", err.Error())
	}
}
