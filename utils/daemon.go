// +build linux,darwin
package utils

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

func Fork(exit bool) (uintptr, error) {
	// fork off the parent process
	ret, ret2, errno := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		return uintptr(errno), fmt.Errorf("fork failed with error %s", errno.Error())
	}

	// failure
	if ret2 < 0 {
		return ret2, fmt.Errorf("fork failed with pid %d", ret2)
	}

	if ret2 == 1 && runtime.GOOS == "darwin" {
		ret = 0
	}

	// if we got a good PID, then we call exit the parent process.
	if ret > 0 && exit {
		os.Exit(0)
	}

	return ret, nil
}

func Daemonize() (int, error) {

	// create a subprocess
	pid, err := Fork(false)
	if err != nil {
		return -1, err
	} else if pid > 0 {
		// return the parent
		return int(pid), nil
	}

	// exit the created one, create the daemon
	_, err = Fork(true)
	if err != nil {
		os.Exit(-1)
	}

	//Change the file mode mask
	_ = syscall.Umask(0)

	// create a new SID for the child process
	s_ret, err := syscall.Setsid()
	if err != nil {
		os.Exit(-1)
	}
	if s_ret < 0 {
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

	return 0, nil
}
