// +build linux

package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/network"
)

const (
	IFNAMSIZ       = 16
	CIFF_TAP       = 0x0002
	CIFF_NO_PI     = 0x1000
	CIFF_ONE_QUEUE = 0x2000
)

type ifReq struct {
	Name  [IFNAMSIZ]byte
	Flags uint16
	pad   [0x28 - 0x10 - 2]byte
}

func GetTapFd(device, bridge, options string) (int, error) {
	var (
		req   ifReq
		errno syscall.Errno
	)

	tapFile, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return -1, err
	}

	req.Flags = CIFF_TAP | CIFF_NO_PI | CIFF_ONE_QUEUE
	copy(req.Name[:len(req.Name)-1], []byte(device))
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, tapFile.Fd(),
		uintptr(syscall.TUNSETIFF),
		uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		tapFile.Close()
		return -1, fmt.Errorf("create tap device failed\n")
	}
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, tapFile.Fd(), uintptr(syscall.TUNSETPERSIST), 0)
	if errno != 0 {
		tapFile.Close()
		return -1, fmt.Errorf("clear tap device persist flag failed\n")
	}

	err = network.UpAndAddToBridge(device, bridge, options)
	if err != nil {
		glog.Errorf("Add to bridge failed %s %s", bridge, device)
		tapFile.Close()
		return -1, err
	}

	return int(tapFile.Fd()), nil
}

func GetVhostUserPort(device, bridge, sockPath, option string) error {
	glog.V(3).Infof("Found ovs bridge %s, attaching tap %s to it\n", bridge, device)
	// append vhost-server-path
	options := fmt.Sprintf("vhost-server-path=%s/%s", sockPath, device)
	if option != "" {
		options = options + "," + option
	}

	// ovs command "ovs-vsctl add-port BRIDGE PORT" add netwok device PORT to BRIDGE,
	// PORT and BRIDGE here indicate the device name respectively.
	out, err := exec.Command("ovs-vsctl", "--may-exist", "add-port", bridge, device, "--", "set", "Interface", device, "type=dpdkvhostuserclient", "options:"+options).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Ovs failed to add port: %s, error :%v", strings.TrimSpace(string(out)), err)
	}

	return nil
}
