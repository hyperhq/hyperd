package system

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// ReadMemInfo retrieves memory statistics of the host system and returns a
//  MemInfo type.
func ReadMemInfo() (*MemInfo, error) {
	meminfo := &MemInfo{}
	stdbuf := new(bytes.Buffer)
	vm_stat, err := exec.LookPath("vm_stat")
	if err != nil {
		return nil, fmt.Errorf("Can not find vm_stat command")
	}

	cmd := exec.Command(vm_stat, "| grep \"Pages free\"")
	cmd.Stdout = stdbuf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	b, _ := stdbuf.ReadBytes(byte('\n'))
	if i := bytes.Index(b, []byte("Pages free:")); i >= 0 {
		b = b[i+11:]
		memFree, _ := strconv.Atoi(strings.Trim(string(b), " "))
		meminfo.MemFree = (int64)(memFree * 4096)
	}
	sysctl, err := exec.LookPath("sysctl")
	if err != nil {
		return nil, fmt.Errorf("Can not find sysctl command")
	}

	cmd = exec.Command(sysctl, "hw.memsize")
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
			if memtotal, err := strconv.ParseUint(string(line)[12:], 0, 64); err != nil {
				continue
			} else {
				meminfo.MemTotal = int64(memtotal)
			}
		}
	}
	return meminfo, nil
}
