package sysinfo

import (
	"io/ioutil"
	"os"
	"strings"
)

// New returns a new SysInfo, using the filesystem to detect which features the kernel supports.
func New(quiet bool) *SysInfo {
	sysInfo := &SysInfo{}

	sysInfo.IPv4ForwardingDisabled = !readProcBool("/proc/sys/net/ipv4/ip_forward")
	sysInfo.BridgeNfCallIptablesDisabled = !readProcBool("/proc/sys/net/bridge/bridge-nf-call-iptables")
	sysInfo.BridgeNfCallIp6tablesDisabled = !readProcBool("/proc/sys/net/bridge/bridge-nf-call-ip6tables")

	// Check if AppArmor is supported.
	if _, err := os.Stat("/sys/kernel/security/apparmor"); !os.IsNotExist(err) {
		sysInfo.AppArmor = true
	}

	return sysInfo
}

func readProcBool(path string) bool {
	val, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(val)) == "1"
}
