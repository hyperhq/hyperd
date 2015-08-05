package virtualbox

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

func GetMediumUUID(disk string) (string, error) {
	if _, err := os.Stat(disk); err != nil {
		return "", err
	}
	var tempdisk string
	if strings.Contains(disk, "private") == true {
		tempdisk = disk[8:]
	} else {
		tempdisk = "/private" + disk
	}
	args := []string{"showmediuminfo", disk}
	info, eOutput, err := vbmOutErr(args...)
	if err != nil {
		args = []string{"showmediuminfo", tempdisk}
		info, eOutput, err = vbmOutErr(args...)
		if err != nil {
			return "", fmt.Errorf(eOutput)
		}
	}
	var uuid string
	reader := bufio.NewReader(strings.NewReader(info))
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if i := bytes.Index(line, []byte("UUID:")); i >= 0 {
			uuid = strings.TrimLeft(string(line)[5:], " ")
			return uuid, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func RegisterDisk(mName, sName, disk string, port int) error {
	machine, err := GetMachine(mName)
	if err != nil {
		return err
	}
	s := StorageMedium{
		Port:      uint(port),
		Device:    0,
		DriveType: DriveHDD,
		MType:     DriveMNormal,
		Medium:    disk,
	}
	if _, stderr, err := machine.AttachStorageWithOutput(sName, s); err != nil {
		if strings.Contains(stderr, "is not found in the media registry") {
			return fmt.Errorf(stderr)
		}
		return err
	}
	s.Medium = "none"
	err = machine.AttachStorage(sName, s)
	if err != nil {
		return err
	}
	return nil
}

func UnregisterDisk(mName, disk string) error {
	uuid, err := GetMediumUUID(disk)
	if err != nil {
		return nil
	}
	args := []string{"closemedium", uuid}
	if err := vbm(args...); err != nil {
		args = []string{"closemedium", disk}
		return vbm(args...)
	}
	return nil
}

func SetNATPF(vmId string, n int, name string, rule PFRule) error {
	return vbm("modifyvm", vmId, fmt.Sprintf("--natpf%d", n),
		fmt.Sprintf("%s,%s", name, rule.Format()))
}
