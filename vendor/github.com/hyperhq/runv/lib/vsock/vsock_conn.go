// Implementation of the net.Conn interface for VsockConn
//
// +build linux

package vsock

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const VsockNetwork = "vsock"

type VsockNetAddr struct {
	cid  uint32
	port uint32
}

func (va *VsockNetAddr) Network() string {
	return VsockNetwork
}

func (va *VsockNetAddr) String() string {
	return fmt.Sprintf("vsock://%d:%d", va.cid, va.port)
}

type VsockConn struct {
	sysFd     int
	readLock  sync.Mutex
	writeLock sync.Mutex

	net          string
	laddr, raddr net.Addr
}

func newVsockConn(fd int, src, dst *unix.SockaddrVM) *VsockConn {
	return &VsockConn{
		sysFd: fd,
		net:   VsockNetwork,
		laddr: &VsockNetAddr{src.CID, src.Port},
		raddr: &VsockNetAddr{dst.CID, dst.Port},
	}
}

func (c *VsockConn) ready() bool { return c != nil && c.sysFd != 0 }

func (c *VsockConn) Read(b []byte) (int, error) {
	var (
		err error
		n   int
	)

	if !c.ready() {
		return 0, syscall.EINVAL
	}

	c.readLock.Lock()
	defer c.readLock.Unlock()
	for {
		n, err = syscall.Read(c.sysFd, b)
		if err != nil {
			n = 0
			if err != syscall.EAGAIN {
				break
			}
		}
		err = io.EOF
		break
	}
	if err != nil && err != io.EOF {
		return 0, &net.OpError{Op: "read", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return n, nil
}

func (c *VsockConn) Write(b []byte) (int, error) {
	var (
		count, n int
		err      error
	)

	if !c.ready() {
		return 0, syscall.EINVAL
	}

	c.writeLock.Lock()
	defer c.writeLock.Unlock()
	for {
		n, err = syscall.Write(c.sysFd, b)
		if n > 0 {
			count += n
		}
		if count == len(b) {
			break
		}

		if err == syscall.EAGAIN {
			continue
		} else if err != nil {
			break
		}
		if n == 0 {
			err = io.ErrUnexpectedEOF
			break
		}
	}
	if err != nil {
		err = &net.OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return count, err
}

func (c *VsockConn) Close() error {
	if !c.ready() {
		return syscall.EINVAL
	}
	err := syscall.Close(c.sysFd)
	if err != nil {
		err = &net.OpError{Op: "close", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return err
}

func (c *VsockConn) LocalAddr() net.Addr {
	if !c.ready() {
		return nil
	}
	return c.laddr
}

func (c *VsockConn) RemoteAddr() net.Addr {
	if !c.ready() {
		return nil
	}
	return c.raddr
}

func (c *VsockConn) SetDeadline(t time.Time) error {
	if !c.ready() {
		return syscall.EINVAL
	}
	return c.setDeadlineImpl(t, 'r'+'w')
}

func (c *VsockConn) SetReadDeadline(t time.Time) error {
	if !c.ready() {
		return syscall.EINVAL
	}
	return c.setDeadlineImpl(t, 'r')
}

func (c *VsockConn) SetWriteDeadline(t time.Time) error {
	if !c.ready() {
		return syscall.EINVAL
	}
	return c.setDeadlineImpl(t, 'w')
}

func (c *VsockConn) File() (f *os.File, err error) {
	defer func() {
		if err != nil {
			err = &net.OpError{Op: "file", Net: c.net, Source: c.laddr, Addr: c.laddr, Err: err}
		}
	}()

	fd, err := syscall.Dup(c.sysFd)
	if err != nil {
		return
	}
	syscall.CloseOnExec(fd)
	if err = syscall.SetNonblock(fd, false); err != nil {
		return
	}

	return os.NewFile(uintptr(fd), c.name()), nil
}

func (c *VsockConn) name() string {
	var l, r string
	if c.laddr != nil {
		l = c.laddr.String()
	}
	if c.raddr != nil {
		r = c.raddr.String()
	}
	return c.net + ":" + l + "->" + r
}

func (c *VsockConn) setDeadlineImpl(t time.Time, mode int) error {
	switch mode {
	case 'r':
	case 'w':
	case 'r' + 'w':
	}
	return syscall.EINVAL
}

func Dial(cid uint32, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	unix.SetNonblock(fd, false)

	dst := &unix.SockaddrVM{CID: cid, Port: port}
	err = unix.Connect(fd, dst)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}

	sa, err := unix.Getsockname(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}

	src, ok := sa.(*unix.SockaddrVM)
	if !ok {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to make vsock connection")
	}

	return newVsockConn(fd, src, dst), nil
}
