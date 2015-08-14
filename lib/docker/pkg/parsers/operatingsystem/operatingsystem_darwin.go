package operatingsystem

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GetOperatingSystem() (string, error) {
	sw_vers, err := exec.LookPath("sw_vers")
	if err != nil {
		return "", fmt.Errorf("Can not find sw_vers")
	}
	cmd := exec.Command(sw_vers)
	stdbuf := new(bytes.Buffer)
	cmd.Stdout = stdbuf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	b, _ := stdbuf.ReadBytes(byte('\n'))
	if i := bytes.Index(b, []byte("ProductName:")); i >= 0 {
		b = b[i+16:]
		return strings.Trim(string(b), " "), nil
	}
	return "", fmt.Errorf("ProductName not found")
}

// No-op on Mac OSX
func IsContainerized() (bool, error) {
	return false, nil
}
