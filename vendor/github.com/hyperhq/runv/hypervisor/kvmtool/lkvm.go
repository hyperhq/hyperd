// +build linux

package kvmtool

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/hyperhq/runv/lib/utils"
)

const (
	KVMTOOL_EXEC = "lkvm"
)

//implement the hypervisor.HypervisorDriver interface
type KvmtoolDriver struct {
	executable string
}

//implement the hypervisor.DriverContext interface
type KvmtoolContext struct {
	driver *KvmtoolDriver
	conPty string
}

func InitDriver() *KvmtoolDriver {
	cmd, err := exec.LookPath(KVMTOOL_EXEC)
	if err != nil {
		return nil
	}

	return &KvmtoolDriver{
		executable: cmd,
	}
}

func (kd *KvmtoolDriver) Name() string {
	return "kvmtool"
}

func (kd *KvmtoolDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &KvmtoolContext{
		driver: kd,
	}
}

func (kd *KvmtoolDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	return &KvmtoolContext{
		driver: kd,
	}, nil
}

func (kd *KvmtoolDriver) SupportLazyMode() bool {
	return false
}

func (kd *KvmtoolDriver) SupportVmSocket() bool {
	return false
}

func arguments(ctx *hypervisor.VmContext) []string {
	boot := ctx.Boot
	memParams := strconv.Itoa(boot.Memory)
	cpuParams := strconv.Itoa(boot.CPU)

	// kvmtool enforce uses ttyS0 as console,
	// hyperstart can only use ttyS1 and ttyS2 as ctl and tty channel.
	args := []string{
		"run", "-k", boot.Kernel, "-i", boot.Initrd, "-m", memParams,
		"-c", cpuParams, "--name", ctx.Id}
	//use ttyS0 as kernel console
	args = append(args, "-p", "iommu=off console=ttyS0 "+json.HYPER_USE_SERIAL)

	// kvmtool enforce uses ttyS0 as console,
	// hyperstart can only use ttyS1 and ttyS2 as ctl and tty channel.
	args = append(args, "--tty", "0", "--tty", "1", "--tty", "2")

	// attach nic at the start since kvmtool doesn't support hotplug
	args = append(args, "--network", "mode=tap,tapif="+network.NicName(ctx.Id, 0))

	// arch specified
	if arch_args := arch_arguments(); arch_args != nil {
		args = append(args, arch_args...)
	}

	// pass ShareDir as rootfs directroy to prevent lkvm share host rootfs with vm
	args = append(args, "-d", ctx.ShareDir)

	// why setup ShareDir as 9p directroy? the 9ptag of rootfs is set to "/dev/root" bydefault
	args = append(args, "--9p", ctx.ShareDir+","+hypervisor.ShareDirTag)

	return args
}

func (kc *KvmtoolContext) Launch(ctx *hypervisor.VmContext) {
	var (
		ptmx    *os.File     = nil
		slaver  *os.File     = nil
		err     error        = nil
		conSock net.Listener = nil
		ctlSock net.Listener = nil
		ttySock net.Listener = nil
	)

	defer func() {
		if err != nil {
			if ptmx != nil {
				ctx.Log(hypervisor.INFO, "close ptmx")
				ptmx.Close()
			}

			if slaver != nil {
				ctx.Log(hypervisor.INFO, "close slaver")
				slaver.Close()
			}

			if conSock != nil {
				ctx.Log(hypervisor.INFO, "close conSock")
				conSock.Close()
			}

			if ctlSock != nil {
				ctx.Log(hypervisor.INFO, "close ctlSock")
				ctlSock.Close()
			}

			if ttySock != nil {
				ctx.Log(hypervisor.INFO, "close ttySock")
				ttySock.Close()
			}

			ctx.Hub <- &hypervisor.VmStartFailEvent{
				Message: fmt.Sprintf("fail to launch kvmtool: %s", err),
			}
		}
	}()

	if kc.driver.executable == "" {
		err = fmt.Errorf("can not find kvmtool executable")
		return
	}

	args := arguments(ctx)

	cmd := exec.Command(kc.driver.executable, args...)

	ptmx, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		ctx.Log(hypervisor.ERROR, "fail to open /dev/ptmx: %v", err)
		return
	}

	var num int = 0
	_, _, ret := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ptmx.Fd()),
		syscall.TIOCGPTN, uintptr(unsafe.Pointer(&num)))
	if ret != 0 {
		err = fmt.Errorf("fail to ioctl /dev/ptmx: %v\n", ret)
		return
	}
	pts := fmt.Sprintf("/dev/pts/%v", num)

	num = 0
	_, _, ret = syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(),
		syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&num)))

	slaver, err = os.OpenFile(pts, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		ctx.Log(hypervisor.ERROR, "fail to open %v: %v\n", pts, err)
		return
	}

	cmd.Stdin = slaver
	cmd.Stdout = slaver
	cmd.Stderr = slaver
	cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}
	cmd.Env = []string{fmt.Sprintf("HOME=%s", hypervisor.BaseDir)}

	err = cmd.Start()
	if err != nil {
		ctx.Log(hypervisor.ERROR, "fail to start kvmtool vm %v %v %v", kc.driver.executable, args, err)
		return
	}

	go func() {
		cmd.Wait()
	}()

	retry := 0
	var output []byte
	len := 0
	for retry < 5 {
		data := make([]byte, 128)
		nr, err := ptmx.Read(data)
		output = append(output, data[:nr]...)
		len += nr
		if nr > 0 {
			conPty, ctlPty, ttyPty := lookupPtys(output[:len])
			ctx.Log(hypervisor.INFO, "find %v %v %v", conPty, ctlPty, ttyPty)
			if conPty != "" && ctlPty != "" && ttyPty != "" {
				ctlSock, err = net.Listen("unix", ctx.HyperSockName)
				if err != nil {
					return
				}
				ttySock, err = net.Listen("unix", ctx.TtySockName)
				if err != nil {
					return
				}

				kc.conPty = conPty
				go sock2pty(ctlSock, ctlPty)
				go sock2pty(ttySock, ttyPty)

				go func() {
					// without this, guest burst output will crash kvmtool, why?
					slaver.Close()
					for true {
						nr, err := ptmx.Read(data)
						if nr > 0 {
							glog.Infof("lkvm output: %v", string(data[:nr]))
						}
						if err != nil {
							glog.Infof("read lkvm output failed: %v", err)
							break
						}
					}
					ptmx.Close()
				}()
				return
			}
		}
		time.Sleep(1 * time.Second)
		retry++
	}
	//TODO: kill lkvm
	err = fmt.Errorf("cannot find pts devices used by lkvm")
}

func sock2pty(ls net.Listener, ptypath string) {
	defer ls.Close()

	conn, err := ls.Accept()
	if err != nil {
		glog.Errorf("fail to accept client %v", err)
		return
	}
	defer func() {
		conn.Close()
		glog.Infof("close socket to pty")
	}()

	pty, err := os.OpenFile(ptypath, os.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		glog.Errorf("fail to open %v, %v", ptypath, err)
		return
	}
	defer pty.Close()

	_, err = term.SetRawTerminal(pty.Fd())
	if err != nil {
		glog.Errorf("fail to setrowmode for %v: %v", ptypath, err)
		return
	}

	wg := &sync.WaitGroup{}

	copy := func(dst io.Writer, src io.Reader, input bool) {
		// copy from io.Copy, kvmtool driver needs to pass the escape_char as normal char
		buf := make([]byte, 32*1024)
		for {
			var newbuf []byte = nil
			nr, er := src.Read(buf)
			if nr > 0 {
				newbuf = buf
				if input {
					newbuf = bytes.Replace(buf[:nr], []byte{1}, []byte{1, 1}, -1)
					nr = len(newbuf)
				}
				nw, ew := dst.Write(newbuf[:nr])
				if ew != nil {
					glog.Infof("write failed: %v", ew)
					break
				}
				if nr != nw {
					glog.Infof("write != read")
					break
				}
				continue
			}
			if er == io.EOF {
				glog.Infof("read end of file")
				break
			}
			if er != nil {
				glog.Infof("read failed: %v", er)
				break
			}
		}

		wg.Done()
		glog.Infof("REACH the END of io copy")
	}

	wg.Add(1)
	go copy(conn, pty, false)

	wg.Add(1)
	go copy(pty, conn, true)

	wg.Wait()
}

func ptySplit(r rune) bool {
	return r == ' ' || r == '\n' || r == '\r'
}

func lookupPtys(data []byte) (string, string, string) {
	con := ""
	ctl := ""
	tty := ""

	fields := bytes.FieldsFunc(data, ptySplit)
	for i, field := range fields {
		if (string(field) == "pty") && (i < len(fields)-1) {
			if con == "" {
				con = string(fields[i+1])
			} else if ctl == "" {
				ctl = string(fields[i+1])
			} else if tty == "" {
				tty = string(fields[i+1])
				break
			}
		}
	}

	return con, ctl, tty
}

func (kc *KvmtoolContext) Associate(ctx *hypervisor.VmContext) {

}

func (kc *KvmtoolContext) Dump() (map[string]interface{}, error) {
	return nil, nil
}

func (kc *KvmtoolContext) Shutdown(ctx *hypervisor.VmContext) {
	ctx.Log(hypervisor.INFO, "shutdown")
	cmd := exec.Command(kc.driver.executable, []string{"stop", "-n", ctx.Id}...)
	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = []string{fmt.Sprintf("HOME=%s", hypervisor.BaseDir)}

	cmd.Run()
}

func (kc *KvmtoolContext) Kill(ctx *hypervisor.VmContext) {
	kc.Shutdown(ctx)
	ctx.Hub <- &hypervisor.VmKilledEvent{Success: true}
}

func (kc *KvmtoolContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (kc *KvmtoolContext) Close() {

}

func (kc *KvmtoolContext) Pause(ctx *hypervisor.VmContext, pause bool) error {
	err := fmt.Errorf("doesn't support pause for kvmtool right now")
	ctx.Log(hypervisor.ERROR, "%v", err)
	return err
}

func scsiId2Name(id int) string {
	return "vd" + utils.DiskId2Name(id)
}

func (kc *KvmtoolContext) AddDisk(ctx *hypervisor.VmContext, sourceType string, blockInfo *hypervisor.DiskDescriptor, result chan<- hypervisor.VmEvent) {
	result <- &hypervisor.BlockdevInsertedEvent{
		DeviceName: scsiId2Name(blockInfo.ScsiId),
	}

	//TODO: mount block root device to sharedir
}

func (kc *KvmtoolContext) RemoveDisk(ctx *hypervisor.VmContext, blockInfo *hypervisor.DiskDescriptor, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	ctx.Log(hypervisor.INFO, "RemoveDisk is unsupported on kvmtool driver")
	result <- callback

}

func (kc *KvmtoolContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	ctx.Log(hypervisor.INFO, "Hotplug is unsupported on kvmtool...")
	// Nic has already attached on lkvm vm, so only add this interface into bridge
	network.UpAndAddToBridge(network.NicName(ctx.Id, 0), host.Bridge, "")
	result <- &hypervisor.NetDevInsertedEvent{
		Id:         host.Id,
		Index:      guest.Index,
		DeviceName: guest.Device,
		Address:    guest.Busaddr,
	}
}

func (kc *KvmtoolContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	ctx.Log(hypervisor.INFO, "RemoveNic is unsupported on kvmtool driver")
	result <- callback
}

func (kc *KvmtoolContext) SetCpus(ctx *hypervisor.VmContext, cpus int) error {
	return fmt.Errorf("SetCpus is unsupported on kvmtool driver")
}

func (kc *KvmtoolContext) AddMem(ctx *hypervisor.VmContext, slot, size int) error {
	return fmt.Errorf("AddMem is unsupported on kvmtool driver")
}

func (kc *KvmtoolContext) Save(ctx *hypervisor.VmContext, path string) error {
	return fmt.Errorf("Save is unsupported on kvmtool driver")
}

func (kc *KvmtoolContext) ConnectConsole(console chan<- string) error {
	pty, err := os.OpenFile(kc.conPty, os.O_RDWR|syscall.O_NOCTTY, 0600)
	if err != nil {
		glog.Errorf("fail to open %v, %v", kc.conPty, err)
		return err
	}

	_, err = term.SetRawTerminal(pty.Fd())
	if err != nil {
		glog.Errorf("fail to setrowmode for %v: %v", kc.conPty, err)
		return err
	}

	go func() {
		data := make([]byte, 128)
		for {
			nr, err := pty.Read(data)
			if err != nil {
				glog.Errorf("fail to read console: %v", err)
				break
			}
			console <- string(data[:nr])
		}
		pty.Close()
	}()

	return nil

}
