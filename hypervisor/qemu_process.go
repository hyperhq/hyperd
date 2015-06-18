package hypervisor

import (
	"encoding/binary"
	"fmt"
	"hyper/lib/glog"
	"hyper/lib/telnet"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func unixSocketConnect(name string) (conn net.Conn, err error) {
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", name)
		if err == nil {
			return
		}
	}

	return
}

func waitConsoleOutput(ctx *VmContext) {

	conn, err := unixSocketConnect(ctx.consoleSockName)
	if err != nil {
		glog.Error("failed to connected to ", ctx.consoleSockName, " ", err.Error())
		return
	}

	glog.V(1).Info("connected to ", ctx.consoleSockName)

	tc, err := telnet.NewConn(conn)
	if err != nil {
		glog.Error("fail to init telnet connection to ", ctx.consoleSockName, ": ", err.Error())
		return
	}
	glog.V(1).Infof("connected %s as telnet mode.", ctx.consoleSockName)

	cout := make(chan string, 128)
	go ttyLiner(tc, cout)

	for {
		line, ok := <-cout
		if ok {
			glog.V(1).Info("[console] ", line)
		} else {
			glog.Info("console output end")
			break
		}
	}
}

func (ctx *VmContext) timedKill(seconds int) {
	ctx.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if ctx != nil && ctx.handler != nil {
			ctx.wdt <- "kill"
		}
	})
}

func watchDog(ctx *VmContext) {
	for {
		msg, ok := <-ctx.wdt
		if ok {
			switch msg {
			case "quit":
				glog.V(1).Info("quit watch dog.")
				return
			case "kill":
				success := false
				if ctx.process != nil {
					glog.V(0).Infof("kill Qemu... %d", ctx.process.Pid)
					if err := ctx.process.Kill(); err == nil {
						success = true
					}
				} else {
					glog.Warning("no process to be killed")
				}
				ctx.hub <- &QemuKilledEvent{success: success}
				return
			}
		} else {
			glog.V(1).Info("chan closed, quit watch dog.")
			break
		}
	}
}

func fork(exit bool) (uintptr, error) {
	// fork off the parent process
	ret, ret2, errno := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		return uintptr(errno), fmt.Errorf("fork failed with error %s", errno.Error())
	}

	// failure
	if ret2 < 0 {
		return ret2, fmt.Errorf("fork failed with pid %d", ret2)
	}

	// if we got a good PID, then we call exit the parent process.
	if ret > 0 && exit {
		glog.V(3).Infof("I am the parent, exit, ps: child %d", ret)
		os.Exit(0)
	}

	return ret, nil
}

func listFd() []string {
	files, err := ioutil.ReadDir("/proc/self/fd/")
	if err != nil {
		return []string{}
	}

	result := []string{}
	for _, file := range files {
		result = append(result, file.Name())
	}

	return result
}

func daemon(cmd string, argv []string, pipe int) error {

	// create a subprocess
	pid, err := fork(false)
	if err != nil {
		return err
	} else if pid > 0 {
		go func() {
			wp, err := syscall.Wait4(int(pid), nil, 0, nil)
			if err == nil {
				glog.V(3).Infof("collect child %d", wp)
			} else {
				glog.Errorf("error during wait %d: %s", pid, err.Error())
			}
		}()
		// return the parent
		return nil
	}

	// exit the created one, create the daemon
	_, err = fork(true)
	if err != nil {
		glog.Error("second fork failed: ", err.Error())
		os.Exit(-1)
	}

	cur := os.Getpid()
	glog.V(1).Infof("qemu daemon pid %d.", cur)
	//Change the file mode mask
	_ = syscall.Umask(0)

	// create a new SID for the child process
	s_ret, err := syscall.Setsid()
	if err != nil {
		glog.Info("Error: syscall.Setsid errno: ", err.Error())
		os.Exit(-1)
	}
	if s_ret < 0 {
		glog.Errorf("setsid return negative value: %d", s_ret)
		os.Exit(-1)
	}

	os.Chdir("/")

	f, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if e == nil {
		fd := f.Fd()
		syscall.Dup2(int(fd), int(os.Stdin.Fd()))
		syscall.Dup2(int(fd), int(os.Stdout.Fd()))
		syscall.Dup2(int(fd), int(os.Stderr.Fd()))
	}

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(cur))
	syscall.Write(pipe, buf)
	syscall.Close(pipe)

	fds := listFd()
	for _, fd := range fds {
		if f, err := strconv.Atoi(fd); err == nil && f > 2 {
			glog.V(1).Infof("close fd %d", f)
			syscall.Close(f)
		}
	}

	err = syscall.Exec(cmd, argv, []string{})
	if err != nil {
		glog.Error("fail to exec qemu process")
		os.Exit(-1)
	}

	return nil
}

func (ctx *VmContext) watchPid(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	ctx.process = proc
	go watchDog(ctx)

	return nil
}

// launchQemu run qemu and wait it's quit, includes
func launchQemu(ctx *VmContext) {
	qemu, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		ctx.hub <- &QemuExitEvent{message: "can not find qemu executable"}
		return
	}

	args := ctx.QemuArguments()

	if glog.V(1) {
		glog.Info("cmdline arguments: ", strings.Join(args, " "))
	}

	go waitConsoleOutput(ctx)

	pipe := make([]int, 2)
	err = syscall.Pipe(pipe)
	if err != nil {
		glog.Error("fail to create pipe")
		ctx.hub <- &QemuExitEvent{message: "fail to create pipe"}
		return
	}

	err = daemon(qemu, append([]string{"qemu-system-x86_64"}, args...), pipe[1])
	if err != nil {
		//fail to daemonize
		glog.Error("try to start qemu failed")
		ctx.hub <- &QemuExitEvent{message: "try to start qemu failed"}
		return
	}

	buf := make([]byte, 4)
	nr, err := syscall.Read(pipe[0], buf)
	if err != nil || nr != 4 {
		glog.Error("try to start qemu failed")
		ctx.hub <- &QemuExitEvent{message: "try to start qemu failed"}
		return
	}
	syscall.Close(pipe[1])
	syscall.Close(pipe[0])

	pid := binary.BigEndian.Uint32(buf[:nr])
	glog.V(1).Infof("starting daemon with pid: %d", pid)

	err = ctx.watchPid(int(pid))
	if err != nil {
		glog.Error("watch qemu process failed")
		ctx.hub <- &QemuExitEvent{message: "watch qemu process failed"}
		return
	}
}

func associateQemu(ctx *VmContext) {
	go waitConsoleOutput(ctx)
	go watchDog(ctx)
}
