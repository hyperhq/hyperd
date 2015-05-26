package sysinfo

type CpuInfo struct {
	Processor              uint64
	Vender_id              string
	CpuFamily              uint64
	Mode                   uint64
	ModeName               string
	Stepping               uint64
	MicroCode              string
	CpuMhz                 float64
	Cache                  string
	PhyId                  uint64
	Siblings               uint64
	CoreId                 uint64
	CpuCores               uint64
	Apicid                 uint64
	InitApicid             uint64
	Fpu                    bool
	FpuException           bool
	CpuidLevel             uint64
	Wp                     bool
	Flags                  string
	Bogomips               float64
	ClFlushSize            uint64
	CacheAlianment         uint64
	AddressSizes           string
	PowerManagement        string
}

type MemInfo struct {
	MemTotal               uint64
	MemFree                uint64
	MemAvailable           uint64
	Buffers                uint64
	Cached                 uint64
	SwapCached             uint64
	Active                 uint64
	Inactive               uint64
	AnonActive             uint64
	AnonInactive           uint64
	FileActive             uint64
	FileInactive           uint64
	Unevictable            uint64
	Mlocked                uint64
	SwapTotal              uint64
	SwapFree               uint64
	Dirty                  uint64
	Writeback              uint64
	AnonPages              uint64
	Mapped                 uint64
	Shmem                  uint64
	Slab                   uint64
	SReclaimable           uint64
	SUnreclaim             uint64
	KernelStack            uint64
	PageTables             uint64
	NFS_Unstable           uint64
	Bounce                 uint64
	WritebackTmp           uint64
	CommitLimit            uint64
	Committed_AS           uint64
	VmallocTotal           uint64
	VmallocUsed            uint64
	VmallocChunk           uint64
	HardwareCorrupted      uint64
	AnonHugePages          uint64
	HugePages_Total        uint64
	HugePages_Free         uint64
	HugePages_Rsvd         uint64
	HugePages_Surp         uint64
	Hugepagesize           uint64
	DirectMap4k            uint64
	DirectMap2M            uint64
	DirectMap1G            uint64
}

type OSInfo struct {
	Name                   string
	Version                string
	Id                     string
	IdLike                 string
	PrettyName             string
	VersionId              string
	HomeURL                string
	SupportURL             string
	BugURL                 string
}

func GetCpuInfo() (*CpuInfo, error) {
	return getCpuInfo()
}

func GetMemInfo() (*MemInfo, error) {
	return getMemInfo()
}

func GetOSInfo() (*OSInfo, error) {
	return getOSInfo()
}
