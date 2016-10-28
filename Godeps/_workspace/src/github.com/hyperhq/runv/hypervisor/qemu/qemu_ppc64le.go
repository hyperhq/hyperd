// +build linux,ppc64le

package qemu

import (
	"fmt"
	"strconv"

	"github.com/hyperhq/runv/hypervisor"
)

const (
	QEMU_SYSTEM_EXE    = "qemu-system-ppc64le"
	VM_MIN_MEMORY_SIZE = 256 // On ppc64le the minimum memory size of a VM is 256 MiB
)

func (qc *QemuContext) arguments(ctx *hypervisor.VmContext) []string {
	if ctx.Boot == nil {
		ctx.Boot = &hypervisor.BootConfig{
			CPU:    1,
			Memory: VM_MIN_MEMORY_SIZE,
			Kernel: hypervisor.DefaultKernel,
			Initrd: hypervisor.DefaultInitrd,
		}
	}
	boot := ctx.Boot
	qc.cpus = boot.CPU

	// Currently the default memory size is fixed to 128 MiB.
	// TODO: Check with PPC team for a better solution
	if boot.Memory < VM_MIN_MEMORY_SIZE {
		boot.Memory = VM_MIN_MEMORY_SIZE
	}

	var memParams, cpuParams string
	if boot.HotAddCpuMem {
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		memParams = strconv.Itoa(boot.Memory)
		cpuParams = strconv.Itoa(boot.CPU)
	}

	return []string{
		"-machine", "pseries,accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host",
		"-kernel", boot.Kernel, "-initrd", boot.Initrd,
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams,
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.ConsoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	}
}
