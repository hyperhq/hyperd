// +build linux,arm64

package qemu

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

const (
	QEMU_SYSTEM_EXE               = "qemu-system-aarch64"
	VM_MIN_MEMORY_SIZE            = 128
	CAVIUM_CPU_PART_THUNDERX      = 0x0A1
	CAVIUM_CPU_PART_THUNDERX_81XX = 0x0A2
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
	if boot.Memory < VM_MIN_MEMORY_SIZE {
		boot.Memory = VM_MIN_MEMORY_SIZE
	}

	var memParams, cpuParams string
	memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", boot.Memory, hypervisor.DefaultMaxMem)
	cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", boot.CPU, hypervisor.DefaultMaxCpus)

	gic_version3 := false
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			infos := strings.FieldsFunc(scanner.Text(), func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsNumber(c)
			})
			if len(infos) == 3 && infos[0] == "CPU" && infos[1] == "part" {
				if partnum, err := strconv.ParseInt(infos[2], 0, 32); err == nil {
					glog.Infof("partnum is %v, cpu type is thunder", partnum)
					if partnum == CAVIUM_CPU_PART_THUNDERX || partnum == CAVIUM_CPU_PART_THUNDERX_81XX {
						gic_version3 = true
					}
				}
				break
			}
		}
	}

	kvm_available := true
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		kvm_available = false
		glog.V(1).Info("kvm not exist change to no kvm mode")
	}

	params := []string{"-machine", "virt,usb=off", "-cpu", "cortex-a57"}
	if gic_version3 {
		params = []string{"-machine", "virt,accel=kvm,gic-version=3,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host"}
		if !kvm_available {
			params = []string{"-machine", "virt,gic-version=3,usb=off", "-cpu", "host"}
		}
	}

	return append(params,
		"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "console=ttyAMA0 panic=1",
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults",
		"-rtc", "base=utc,clock=vm,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams,
		"-device", "pci-bridge,chassis_nr=1,id=pci.0",
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.ConsoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	)
}
