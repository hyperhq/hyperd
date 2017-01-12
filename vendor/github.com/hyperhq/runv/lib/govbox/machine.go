package virtualbox

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

type MachineState string

const (
	Poweroff = MachineState("poweroff")
	Running  = MachineState("running")
	Paused   = MachineState("paused")
	Saved    = MachineState("saved")
	Aborted  = MachineState("aborted")
)

type Flag int

// Flag names in lowercases to be consistent with VBoxManage options.
const (
	F_acpi Flag = 1 << iota
	F_ioapic
	F_rtcuseutc
	F_cpuhotplug
	F_pae
	F_longmode
	F_synthcpu
	F_hpet
	F_hwvirtex
	F_triplefaultreset
	F_nestedpaging
	F_largepages
	F_vtxvpid
	F_vtxux
	F_accelerate3d
)

// Convert bool to "on"/"off"
func bool2string(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Test if flag is set. Return "on" or "off".
func (f Flag) Get(o Flag) string {
	return bool2string(f&o == o)
}

// Machine information.
type Machine struct {
	Name       string
	UUID       string
	State      MachineState
	CPUs       uint
	Memory     uint // main memory (in MB)
	VRAM       uint // video memory (in MB)
	CfgFile    string
	BaseFolder string
	OSType     string
	Flag       Flag
	BootOrder  []string // max 4 slots, each in {none|floppy|dvd|disk|net}
}

// Refresh reloads the machine information.
func (m *Machine) Refresh() error {
	id := m.Name
	if id == "" {
		id = m.UUID
	}
	mm, err := GetMachine(id)
	if err != nil {
		return err
	}
	*m = *mm
	return nil
}

// Start starts the machine.
func (m *Machine) Start() error {
	switch m.State {
	case Paused:
		return vbm("controlvm", m.Name, "resume")
	case Poweroff, Saved, Aborted:
		if glog.V(4) {
			return vbm("startvm", m.Name, "--type", "gui")
		}
		return vbm("startvm", m.Name, "--type", "headless")
	}
	return nil
}

// Suspend suspends the machine and saves its state to disk.
func (m *Machine) Save() error {
	switch m.State {
	case Paused:
		if err := m.Start(); err != nil {
			return err
		}
	case Poweroff, Aborted, Saved:
		return nil
	}
	return vbm("controlvm", m.Name, "savestate")
}

// Pause pauses the execution of the machine.
func (m *Machine) Pause() error {
	switch m.State {
	case Paused, Poweroff, Aborted, Saved:
		return nil
	}
	return vbm("controlvm", m.Name, "pause")
}

// Stop gracefully stops the machine.
func (m *Machine) Stop() error {
	switch m.State {
	case Poweroff, Aborted, Saved:
		return nil
	case Paused:
		if err := m.Start(); err != nil {
			return err
		}
	}

	for m.State != Poweroff { // busy wait until the machine is stopped
		if err := vbm("controlvm", m.Name, "acpipowerbutton"); err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
		if err := m.Refresh(); err != nil {
			return err
		}
	}
	return nil
}

// Poweroff forcefully stops the machine. State is lost and might corrupt the disk image.
func (m *Machine) Poweroff() error {
	switch m.State {
	case Poweroff, Aborted, Saved:
		glog.Warningf("The machine status is Poweroff, Aborted, Saved")
		return nil
	}
	return vbm("controlvm", m.Name, "poweroff")
}

// Restart gracefully restarts the machine.
func (m *Machine) Restart() error {
	switch m.State {
	case Paused, Saved:
		if err := m.Start(); err != nil {
			return err
		}
	}
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

// Reset forcefully restarts the machine. State is lost and might corrupt the disk image.
func (m *Machine) Reset() error {
	switch m.State {
	case Paused, Saved:
		if err := m.Start(); err != nil {
			return err
		}
	}
	return vbm("controlvm", m.Name, "reset")
}

// Delete deletes the machine and associated disk images.
func (m *Machine) Delete() error {
	if err := m.Poweroff(); err != nil {
		// Ignore this error
	}
	//	return vbm("unregistervm", m.Name, "--delete")
	return vbm("unregistervm", m.Name)
}

// GetMachine finds a machine by its name or UUID.
func GetMachine(id string) (*Machine, error) {
	stdout, stderr, err := vbmOutErr("showvminfo", id, "--machinereadable")
	if err != nil {
		if reMachineNotFound.FindString(stderr) != "" {
			return nil, ErrMachineNotExist
		}
		return nil, err
	}
	s := bufio.NewScanner(strings.NewReader(stdout))
	m := &Machine{}
	for s.Scan() {
		res := reVMInfoLine.FindStringSubmatch(s.Text())
		if res == nil {
			continue
		}
		key := res[1]
		if key == "" {
			key = res[2]
		}
		val := res[3]
		if val == "" {
			val = res[4]
		}

		switch key {
		case "name":
			m.Name = val
		case "UUID":
			m.UUID = val
		case "VMState":
			m.State = MachineState(val)
		case "memory":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.Memory = uint(n)
		case "cpus":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.CPUs = uint(n)
		case "vram":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.VRAM = uint(n)
		case "CfgFile":
			m.CfgFile = val
			m.BaseFolder = filepath.Dir(val)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

// ListMachines lists all registered machines.
func ListMachines() ([]*Machine, error) {
	out, err := vbmOut("list", "vms")
	if err != nil {
		return nil, err
	}
	ms := []*Machine{}
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		res := reVMNameUUID.FindStringSubmatch(s.Text())
		if res == nil {
			continue
		}
		m, err := GetMachine(res[1])
		if err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return ms, nil
}

// CreateMachine creates a new machine. If basefolder is empty, use default.
func CreateMachine(name, basefolder string) (*Machine, error) {
	if name == "" {
		return nil, fmt.Errorf("machine name is empty")
	}

	// Create and register the machine.
	args := []string{"createvm", "--name", name, "--register"}
	if basefolder != "" {
		args = append(args, "--basefolder", basefolder)
	}
	if err := vbm(args...); err != nil {
		return nil, err
	}

	m, err := GetMachine(name)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Modify changes the settings of the machine.
func (m *Machine) Modify(extra []string) error {
	args := []string{"modifyvm", m.Name,
		"--firmware", "bios",
		"--bioslogofadein", "off",
		"--bioslogofadeout", "off",
		"--bioslogodisplaytime", "0",
		"--biosbootmenu", "disabled",

		"--ostype", m.OSType,
		"--cpus", fmt.Sprintf("%d", m.CPUs),
		"--memory", fmt.Sprintf("%d", m.Memory),
		"--vram", fmt.Sprintf("%d", m.VRAM),

		"--acpi", m.Flag.Get(F_acpi),
		"--ioapic", m.Flag.Get(F_ioapic),
		"--rtcuseutc", m.Flag.Get(F_rtcuseutc),
		"--cpuhotplug", m.Flag.Get(F_cpuhotplug),
		"--pae", m.Flag.Get(F_pae),
		"--longmode", m.Flag.Get(F_longmode),
		"--hpet", m.Flag.Get(F_hpet),
		"--hwvirtex", m.Flag.Get(F_hwvirtex),
		"--triplefaultreset", m.Flag.Get(F_triplefaultreset),
		"--nestedpaging", m.Flag.Get(F_nestedpaging),
		"--largepages", m.Flag.Get(F_largepages),
		"--vtxvpid", m.Flag.Get(F_vtxvpid),
		"--vtxux", m.Flag.Get(F_vtxux),
		"--accelerate3d", m.Flag.Get(F_accelerate3d),
	}

	for i, dev := range m.BootOrder {
		if i > 3 {
			break // Only four slots `--boot{1,2,3,4}`. Ignore the rest.
		}
		args = append(args, fmt.Sprintf("--boot%d", i+1), dev)
	}

	if len(extra) > 0 {
		args = append(args, extra...)
	}
	if err := vbm(args...); err != nil {
		return err
	}
	return m.Refresh()
}

// AddNATPF adds a NAT port forarding rule to the n-th NIC with the given name.
func (m *Machine) AddNATPF(n int, name string, rule PFRule) error {
	return vbm("controlvm", m.Name, fmt.Sprintf("natpf%d", n),
		fmt.Sprintf("%s,%s", name, rule.Format()))
}

// DelNATPF deletes the NAT port forwarding rule with the given name from the n-th NIC.
func (m *Machine) DelNATPF(n int, name string) error {
	return vbm("controlvm", m.Name, fmt.Sprintf("natpf%d", n), "delete", name)
}

func (m *Machine) NicConf(n int, nic NIC) []string {
	args := []string{
		fmt.Sprintf("--nic%d", n), string(nic.Network),
		fmt.Sprintf("--nictype%d", n), string(nic.Hardware),
	}

	if nic.Network != "null" {
		args = append(args, fmt.Sprintf("--cableconnected%d", n))
		args = append(args, "on")
	}
	if nic.Network == "hostonly" {
		args = append(args, fmt.Sprintf("--hostonlyadapter%d", n), nic.HostonlyAdapter)
	}
	if nic.Network == "bridged" {
		args = append(args, fmt.Sprintf("--bridgeadapter%d", n), nic.BridgedAdapter)
	}
	if nic.Network == "nat" {
		args = append(args, fmt.Sprintf("--natdnshostresolver%d", n), "on")
	}

	if nic.NatNet != "" {
		args = append(args, fmt.Sprintf("--natnet%d", n), nic.NatNet)
	}

	return args
}

// SetNIC set the n-th NIC.
func (m *Machine) SetNIC(n int, nic NIC) error {
	args := m.NicConf(n, nic)
	args = append([]string{"modifyvm", m.Name}, args...)
	return vbm(args...)
}

// SetNIC set the n-th NIC.
func (m *Machine) ModifyNIC(n int, nicType NICNetwork, device string) error {
	args := []string{"controlvm", m.Name,
		fmt.Sprintf("nic%d", n)}
	if (string)(nicType) == string(NICNetNAT) {
		args = append(args, "nat")
	} else if device != "" {
		args = append(args, (string)(nicType), device)
	} else {
		args = append(args, (string)(nicType))
	}
	return vbm(args...)
}

// DelNIC disable the n-th NIC.
func (m *Machine) DelNIC(n int) error {
	args := []string{"controlvm", m.Name,
		fmt.Sprintf("nic%d null", n),
	}

	return vbm(args...)
}

// AddStorageCtl adds a storage controller with the given name.
func (m *Machine) AddStorageCtl(name string, ctl StorageController) error {
	args := []string{"storagectl", m.Name, "--name", name}
	if ctl.SysBus != "" {
		args = append(args, "--add", string(ctl.SysBus))
	}
	if ctl.Ports > 0 {
		args = append(args, "--portcount", fmt.Sprintf("%d", ctl.Ports))
	}
	if ctl.Chipset != "" {
		args = append(args, "--controller", string(ctl.Chipset))
	}
	args = append(args, "--hostiocache", bool2string(ctl.HostIOCache))
	args = append(args, "--bootable", bool2string(ctl.Bootable))
	return vbm(args...)
}

// DelStorageCtl deletes the storage controller with the given name.
func (m *Machine) DelStorageCtl(name string) error {
	return vbm("storagectl", m.Name, "--name", name, "--remove")
}

// AttachStorage attaches a storage medium to the named storage controller.
func (m *Machine) AttachStorage(ctlName string, medium StorageMedium) error {
	args := []string{"storageattach", m.Name, "--storagectl", ctlName,
		"--port", fmt.Sprintf("%d", medium.Port),
		"--device", fmt.Sprintf("%d", medium.Device),
		"--type", string(medium.DriveType),
		"--medium", medium.Medium,
	}

	if medium.SSD {
		args = append(args, "--nonrotational", "on")
	}

	if medium.MType != "" {
		args = append(args, "--mtype", string(medium.MType))
	}

	return vbm(args...)
}

// AttachStorage attaches a storage medium to the named storage controller.
func (m *Machine) AttachStorageWithOutput(ctlName string, medium StorageMedium) (string, string, error) {
	args := []string{"storageattach", m.Name, "--storagectl", ctlName,
		"--port", fmt.Sprintf("%d", medium.Port),
		"--device", fmt.Sprintf("%d", medium.Device),
		"--type", string(medium.DriveType),
		"--medium", medium.Medium,
	}

	if medium.SSD {
		args = append(args, "--nonrotational", "on")
	}

	if medium.MType != "" {
		args = append(args, "--mtype", string(medium.MType))
	}

	return vbmOutErr(args...)
}

// AddSharedFolder adds a shared folder with the given machine
func (m *Machine) AddSharedFolder(name string, sFolder SharedFolder) error {
	if sFolder.Name == "" {
		return fmt.Errorf("shared folder's name can not be null")
	}
	if sFolder.Path == "" {
		return fmt.Errorf("shared folder's path can not be null")
	}
	if _, err := os.Stat(sFolder.Path); err != nil {
		return err
	}
	args := []string{"sharedfolder", "add", m.Name, "--name", sFolder.Name, "--hostpath", sFolder.Path}
	if sFolder.Transient {
		args = append(args, "--transient")
	}
	if sFolder.Readonly {
		args = append(args, "--readonly")
	}
	if sFolder.Automount {
		args = append(args, "--automount")
	}
	return vbm(args...)
}

// DelSharedFolder deletes a shared folder with the given machine
func (m *Machine) RemoveSharedFolder(name, shareFolderName string) error {
	return vbm("sharedfolder", "remove", m.Name, "--name", shareFolderName)
}
