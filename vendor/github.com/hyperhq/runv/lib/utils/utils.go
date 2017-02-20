package utils

import (
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/hyperhq/runv/lib/vsock"
)

const (
	UNIX_SOCKET_PREFIX  = "unix://"
	VSOCK_SOCKET_PREFIX = "vsock://"
)

func DiskId2Name(id int) string {
	var ch byte = 'a' + byte(id%26)
	if id < 26 {
		return string(ch)
	}
	return DiskId2Name(id/26-1) + string(ch)
}

func SocketConnect(addr string) (net.Conn, error) {
	switch {
	case strings.HasPrefix(addr, UNIX_SOCKET_PREFIX):
		return UnixSocketConnect(addr[len(UNIX_SOCKET_PREFIX):])
	case strings.HasPrefix(addr, VSOCK_SOCKET_PREFIX):
		return vmSocketConnect(addr[len(VSOCK_SOCKET_PREFIX):])
	default:
		return nil, fmt.Errorf("unsupported destination: %s", addr)
	}
}

func UnixSocketConnect(addr string) (conn net.Conn, err error) {
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", addr)
		if err == nil {
			return
		}
	}

	return
}

func vmSocketConnect(addr string) (conn net.Conn, err error) {
	seq := strings.Split(addr, ":")
	if len(seq) != 2 {
		return nil, fmt.Errorf("invalid vsock destination: %v", VSOCK_SOCKET_PREFIX+addr)
	}
	cid, err := strconv.ParseUint(seq[0], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid vsock destination: %v", VSOCK_SOCKET_PREFIX+addr)
	}
	port, err := strconv.ParseUint(seq[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid vsock destination: %v", VSOCK_SOCKET_PREFIX+addr)
	}

	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = vsock.Dial(uint32(cid), uint32(port))
		if err == nil {
			return
		}
	}

	return
}

func RandStr(strSize int, randType string) string {
	var dictionary string
	if randType == "alphanum" {
		dictionary = "0123456789abcdefghijklmnopqrstuvwxyz"
	}

	if randType == "alpha" {
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}

	if randType == "number" {
		dictionary = "0123456789"
	}

	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for k, v := range bytes {
		bytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(bytes)
}
