// +build linux,s390x

package qemu

import (
	"fmt"
	"strconv"

	"github.com/hyperhq/runv/hypervisor"
)

const (
	QEMU_SYSTEM_EXE = "qemu-system-s390x"
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

	var memParams, cpuParams string
	if boot.HotAddCpuMem {
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		memParams = strconv.Itoa(boot.Memory)
		cpuParams = strconv.Itoa(boot.CPU)
	}

	return []string{
		"-machine", "s390-ccw-virtio,accel=kvm,usb=off", "-cpu", "host",
		"-kernel", boot.Kernel, "-initrd", boot.Initrd,
		"-append", "\"console=ttyS1 panic=1 no_timer_check\"",
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-enable-kvm",
		"-rtc", "base=utc,clock=vm,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams,
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName),
		"-chardev", fmt.Sprintf("socket,id=charconsole0,path=%s,server,nowait", ctx.ConsoleSockName),
		"-device", "sclpconsole,chardev=charconsole0",
		"-device", "virtio-serial-ccw,id=virtio-serial0",
		"-device", "virtio-scsi-ccw,id=scsi0",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-ccw,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	}

}
