package sysinfo

// SysInfo stores information about which features a kernel supports.
type SysInfo struct {
	MemoryLimit                   bool
	SwapLimit                     bool
	CpuCfsPeriod                  bool
	CpuCfsQuota                   bool
	AppArmor                      bool
	OomKillDisable                bool
	IPv4ForwardingDisabled        bool
	BridgeNfCallIptablesDisabled  bool
	BridgeNfCallIp6tablesDisabled bool
}
