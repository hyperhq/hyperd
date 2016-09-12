package utils

import (
	"net"
	"time"
)

func DiskId2Name(id int) string {
	var ch byte = 'a' + byte(id%26)
	if id < 26 {
		return string(ch)
	}
	return DiskId2Name(id/26-1) + string(ch)
}

func UnixSocketConnect(name string) (conn net.Conn, err error) {
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", name)
		if err == nil {
			return
		}
	}

	return
}
