package hypervisor

import (
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/lib/telnet"
	"github.com/hyperhq/runv/lib/utils"
)

func watchVmConsole(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.ConsoleSockName)
	if err != nil {
		glog.Error("failed to connected to ", ctx.ConsoleSockName, " ", err.Error())
		return
	}

	glog.V(1).Info("connected to ", ctx.ConsoleSockName)

	tc, err := telnet.NewConn(conn)
	if err != nil {
		glog.Error("fail to init telnet connection to ", ctx.ConsoleSockName, ": ", err.Error())
		return
	}
	glog.V(1).Infof("connected %s as telnet mode.", ctx.ConsoleSockName)

	cout := make(chan string, 128)
	go TtyLiner(tc, cout)

	const ignoreLines = 128
	for consoleLines := 0; consoleLines < ignoreLines; consoleLines++ {
		line, ok := <-cout
		if ok {
			ctx.Log(EXTRA, "[CNL] %s", line)
		} else {
			ctx.Log(INFO, "console output end")
			return
		}
	}
	if !ctx.LogLevel(EXTRA) {
		ctx.Log(DEBUG, "[CNL] omit the first %d line of console logs", ignoreLines)
	}
	for {
		line, ok := <-cout
		if ok {
			ctx.Log(DEBUG, "[CNL] %s", line)
		} else {
			ctx.Log(INFO, "console output end")
			return
		}
	}
}

func TtyLiner(conn io.Reader, output chan string) {
	buf := make([]byte, 1)
	line := []byte{}
	cr := false
	emit := false
	for {

		nr, err := conn.Read(buf)
		if err != nil || nr < 1 {
			glog.V(1).Info("Input byte chan closed, close the output string chan")
			close(output)
			return
		}
		switch buf[0] {
		case '\n':
			emit = !cr
			cr = false
		case '\r':
			emit = true
			cr = true
		default:
			cr = false
			line = append(line, buf[0])
		}
		if emit {
			output <- string(line)
			line = []byte{}
			emit = false
		}
	}
}
