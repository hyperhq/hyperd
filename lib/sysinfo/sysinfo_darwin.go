// +build darwin

package sysinfo

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"syscall"
)

func getMemInfo() (*MemInfo, error) {
	mem := &MemInfo{}
	stdbuf := new(bytes.Buffer)
	sysctl, err := exec.LookPath("sysctl")
	if err != nil {
		return nil, fmt.Errorf("Can not find sysctl command")
	}

	cmd := exec.Command(sysctl, "hw.memsize")
	cmd.Stdout = stdbuf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(stdbuf)
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if i := bytes.Index(line, []byte("hw.memsize:")); i >= 0 {
			mem.MemTotal, err = strconv.ParseUint(string(line)[12:], 0, 64)
			if err != nil {
				continue
			}
		}
	}
	return mem, nil
}

func getCpuInfo() (*CpuInfo, error) {
	return nil, nil
}

func getOSInfo() (*OSInfo, error) {
	osinfo := &OSInfo{}
	var err error
	osinfo.Name, err = syscall.Sysctl("kern.ostype")
	if err != nil {
		return nil, err
	}
	osinfo.Version, err = syscall.Sysctl("kern.osrelease")
	if err != nil {
		return nil, err
	}
	osinfo.Id = osinfo.Name
	osinfo.IdLike = osinfo.Name
	osinfo.PrettyName = osinfo.Name + " Kernel Version " + osinfo.Version
	osinfo.VersionId = osinfo.Version
	osinfo.HomeURL = "www.apple.com"
	return osinfo, nil
}
