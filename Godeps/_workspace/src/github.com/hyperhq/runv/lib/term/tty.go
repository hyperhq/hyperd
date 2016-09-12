package term

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

func TtySplice(conn net.Conn) (int, error) {
	inFd, _ := GetFdInfo(os.Stdin)
	oldState, err := SetRawTerminal(inFd)
	if err != nil {
		return -1, err
	}
	defer RestoreTerminal(inFd, oldState)

	br := bufio.NewReader(conn)

	receiveStdout := make(chan error, 1)
	go func() {
		_, err = io.Copy(os.Stdout, br)
		receiveStdout <- err
	}()

	go func() {
		written, err := io.Copy(conn, os.Stdin)
		fmt.Printf("copy from stdin to remote %v, err %v\n", written, err)

		if sock, ok := conn.(interface {
			CloseWrite() error
		}); ok {
			if err := sock.CloseWrite(); err != nil {
				fmt.Printf("Couldn't send EOF: %s\n", err.Error())
			}
		}
	}()

	if err := <-receiveStdout; err != nil {
		return -1, err
	}

	return 0, nil

}
