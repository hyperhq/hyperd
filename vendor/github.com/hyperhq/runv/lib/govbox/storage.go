package virtualbox

// StorageController represents a virtualized storage controller.
type StorageController struct {
	SysBus      SystemBus
	Ports       uint // SATA port count 1--30
	Chipset     StorageControllerChipset
	HostIOCache bool
	Bootable    bool
}

// SystemBus represents the system bus of a storage controller.
type SystemBus string

const (
	SysBusIDE    = SystemBus("ide")
	SysBusSATA   = SystemBus("sata")
	SysBusSCSI   = SystemBus("scsi")
	SysBusSAS    = SystemBus("sas")
	SysBusFloppy = SystemBus("floppy")
)

// StorageControllerChipset represents the hardware of a storage controller.
type StorageControllerChipset string

const (
	CtrlLSILogic    = StorageControllerChipset("LSILogic")
	CtrlLSILogicSAS = StorageControllerChipset("LSILogicSAS")
	CtrlBusLogic    = StorageControllerChipset("BusLogic")
	CtrlIntelAHCI   = StorageControllerChipset("IntelAHCI")
	CtrlPIIX3       = StorageControllerChipset("PIIX3")
	CtrlPIIX4       = StorageControllerChipset("PIIX4")
	CtrlICH6        = StorageControllerChipset("ICH6")
	CtrlI82078      = StorageControllerChipset("I82078")
)

// StorageMedium represents the storage medium attached to a storage controller.
type StorageMedium struct {
	Port      uint
	Device    uint
	DriveType DriveType
	Medium    string // none|emptydrive|<uuid>|<filename|host:<drive>|iscsi
	MType     DriveMType
	SSD       bool
}

// DriveType represents the hardware type of a drive.
type DriveType string

const (
	DriveDVD = DriveType("dvddrive")
	DriveHDD = DriveType("hdd")
	DriveFDD = DriveType("fdd")
)

type DriveMType string

const (
	DriveMNormal       = DriveMType("normal")
	DriveMWritethrough = DriveMType("writethrough")
	DriveMImmutable    = DriveMType("immutable")
	DriveMShareable    = DriveMType("shareable")
	DriveMReadonly     = DriveMType("readonly")
	DriveMMultiattach  = DriveMType("multiattach")
)
