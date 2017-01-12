package virtualbox

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/golang/glog"
)

var (
	VBM string // Path to VBoxManage utility.
)

func init() {
	VBM = "VBoxManage"
	if p := os.Getenv("VBOX_INSTALL_PATH"); p != "" && runtime.GOOS == "windows" {
		VBM = filepath.Join(p, "VBoxManage.exe")
	}
}

var (
	reVMNameUUID      = regexp.MustCompile(`"(.+)" {([0-9a-f-]+)}`)
	reVMInfoLine      = regexp.MustCompile(`(?:"(.+)"|(.+))=(?:"(.*)"|(.*))`)
	reColonLine       = regexp.MustCompile(`(.+):\s+(.*)`)
	reMachineNotFound = regexp.MustCompile(`Could not find a registered machine named '(.+)'`)
)

var (
	ErrMachineExist    = errors.New("machine already exists")
	ErrMachineNotExist = errors.New("machine does not exist")
	ErrVBMNotFound     = errors.New("VBoxManage not found")
)

func vbm(args ...string) error {
	cmd := exec.Command(VBM, args...)
	glog.V(2).Infof("executing: %v %v", VBM, strings.Join(args, " "))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.Error); ok && ee == exec.ErrNotFound {
			return ErrVBMNotFound
		}
		return err
	}
	return nil
}

func vbmOut(args ...string) (string, error) {
	cmd := exec.Command(VBM, args...)
	glog.V(2).Infof("executing: %v %v", VBM, strings.Join(args, " "))

	b, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && ee == exec.ErrNotFound {
			err = ErrVBMNotFound
		}
	}
	return string(b), err
}

func vbmOutErr(args ...string) (string, string, error) {
	cmd := exec.Command(VBM, args...)
	glog.V(2).Infof("executing: %v %v", VBM, strings.Join(args, " "))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && ee == exec.ErrNotFound {
			err = ErrVBMNotFound
		}
	}
	return stdout.String(), stderr.String(), err
}
