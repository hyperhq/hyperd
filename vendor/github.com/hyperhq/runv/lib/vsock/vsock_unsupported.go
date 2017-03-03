// +build darwin dragonfly freebsd netbsd openbsd solaris

package vsock

import (
	"fmt"
	"net"
)

func Dial(cid uint32, port uint32) (net.Conn, error) {
	return nil, fmt.Errorf("vsock is not supported")
}
