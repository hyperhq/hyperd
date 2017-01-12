// +build linux,amd64

package qemu

import (
	"fmt"
	"os"
	"strconv"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

const (
	QEMU_SYSTEM_EXE = "qemu-system-x86_64"
)

func (qc *QemuContext) arguments(ctx *hypervisor.VmContext) []string {
	if ctx.Boot == nil {
		ctx.Boot = &hypervisor.BootConfig{
			CPU:    1,
			Memory: 128,
			Kernel: hypervisor.DefaultKernel,
			Initrd: hypervisor.DefaultInitrd,
		}
	}
	boot := ctx.Boot
	qc.cpus = boot.CPU

	var machineClass, memParams, cpuParams string
	if boot.HotAddCpuMem || boot.BootToBeTemplate || boot.BootFromTemplate {
		machineClass = "pc-i440fx-2.1"
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		machineClass = "pc-i440fx-2.0"
		memParams = strconv.Itoa(boot.Memory)
		cpuParams = strconv.Itoa(boot.CPU)
	}

	params := []string{
		"-machine", machineClass + ",accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host"}
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		glog.V(1).Info("kvm not exist change to no kvm mode")
		params = []string{"-machine", machineClass + ",usb=off", "-cpu", "core2duo"}
	}

	if boot.Bios != "" && boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Bios),
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else if boot.Bios != "" {
		params = append(params,
			"-bios", boot.Bios,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "console=ttyS0 panic=1 no_timer_check")
	} else if boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else {
		params = append(params,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "console=ttyS0 panic=1 no_timer_check")
	}

	params = append(params,
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-no-hpet",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams)

	if boot.BootToBeTemplate || boot.BootFromTemplate {
		memObject := fmt.Sprintf("memory-backend-file,id=hyper-template-memory,size=%dM,mem-path=%s", boot.Memory, boot.MemoryPath)
		if boot.BootToBeTemplate {
			memObject = memObject + ",share=on"
		}
		nodeConfig := fmt.Sprintf("node,nodeid=0,cpus=0-%d,memdev=hyper-template-memory", hypervisor.DefaultMaxCpus-1)
		params = append(params, "-object", memObject, "-numa", nodeConfig)
		if boot.BootFromTemplate {
			params = append(params, "-S", "-incoming", fmt.Sprintf("exec:cat %s", boot.DevicesStatePath))
		}
	} else if boot.HotAddCpuMem {
		nodeConfig := fmt.Sprintf("node,nodeid=0,cpus=0-%d,mem=%d", hypervisor.DefaultMaxCpus-1, boot.Memory)
		params = append(params, "-numa", nodeConfig)
	}

	return append(params, "-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.ConsoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	)
}
