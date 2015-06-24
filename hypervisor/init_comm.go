package hypervisor

import (
	"encoding/binary"
	"fmt"
	"hyper/lib/glog"
	"hyper/lib/telnet"
	"net"
	"time"
)

// Message
type DecodedMessage struct {
	code    uint32
	message []byte
}

type FinishCmd struct {
	Seq uint64 `json:"seq"`
}

func waitConsoleOutput(ctx *VmContext) {

	conn, err := UnixSocketConnect(ctx.ConsoleSockName)
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

func newVmMessage(m *DecodedMessage) []byte {
	length := len(m.message) + 8
	msg := make([]byte, length)
	binary.BigEndian.PutUint32(msg[:], uint32(m.code))
	binary.BigEndian.PutUint32(msg[4:], uint32(length))
	copy(msg[8:], m.message)
	return msg
}

func readVmMessage(conn *net.UnixConn) (*DecodedMessage, error) {
	needRead := 8
	length := 0
	read := 0
	buf := make([]byte, 512)
	res := []byte{}
	for read < needRead {
		want := needRead - read
		if want > 512 {
			want = 512
		}
		glog.V(1).Infof("trying to read %d bytes", want)
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read init data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		glog.V(1).Infof("read %d/%d [length = %d]", read, needRead, length)

		if length == 0 && read >= 8 {
			length = int(binary.BigEndian.Uint32(res[4:8]))
			glog.V(1).Infof("data length is %d", length)
			if length > 8 {
				needRead = length
			}
		}
	}

	return &DecodedMessage{
		code:    binary.BigEndian.Uint32(res[:4]),
		message: res[8:],
	}, nil
}

func waitInitReady(ctx *VmContext) {
	conn, err := UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot connect to hyper socket " + err.Error(),
		}
		return
	}

	glog.Info("Wating for init messages...")

	msg, err := readVmMessage(conn.(*net.UnixConn))
	if err != nil {
		glog.Error("read init message failed... ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "read init message failed... " + err.Error(),
		}
		conn.Close()
	} else if msg.code == INIT_READY {
		glog.Info("Get init ready message")
		ctx.Hub <- &InitConnectedEvent{conn: conn.(*net.UnixConn)}
		go waitCmdToInit(ctx, conn.(*net.UnixConn))
	} else {
		glog.Warningf("Get init message %d", msg.code)
		ctx.Hub <- &InitFailedEvent{
			Reason: fmt.Sprintf("Get init message %d", msg.code),
		}
		conn.Close()
	}
}

func connectToInit(ctx *VmContext) {
	conn, err := UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot re-connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot re-connect to hyper socket " + err.Error(),
		}
		return
	}

	go waitCmdToInit(ctx, conn.(*net.UnixConn))
}

func waitCmdToInit(ctx *VmContext, init *net.UnixConn) {
	looping := true
	cmds := []*DecodedMessage{}

	var pingTimer *time.Timer = nil
	var pongTimer *time.Timer = nil

	go waitInitAck(ctx, init)

	for looping {
		cmd, ok := <-ctx.vm
		if !ok {
			glog.Info("vm channel closed, quit")
			break
		}
		if cmd.code == INIT_ACK || cmd.code == INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].code == INIT_DESTROYPOD {
					glog.Info("got response of shutdown command, last round of command to init")
					looping = false
				}
				if cmd.code == INIT_ACK {
					if cmds[0].code != INIT_PING {
						ctx.Hub <- &CommandAck{
							reply: cmds[0].code,
							msg:   cmd.message,
						}
					}
				} else {
					ctx.Hub <- &CommandError{
						context: cmds[0],
						msg:     cmd.message,
					}
				}
				cmds = cmds[1:]

				if pongTimer != nil {
					glog.V(1).Info("ack got, clear pong timer")
					pongTimer.Stop()
					pongTimer = nil
				}
				if pingTimer == nil {
					pingTimer = time.AfterFunc(30*time.Second, func() {
						defer func() { recover() }()
						glog.V(1).Info("Send ping message to init")
						ctx.vm <- &DecodedMessage{
							code:    INIT_PING,
							message: []byte{},
						}
						pingTimer = nil
					})
				} else {
					pingTimer.Reset(30 * time.Second)
				}
			} else {
				glog.Error("got ack but no command in queue")
			}
		} else if cmd.code == INIT_FINISHPOD {
			num := len(cmd.message) / 4
			results := make([]uint32, num)
			for i := 0; i < num; i++ {
				results[i] = binary.BigEndian.Uint32(cmd.message[i*4 : i*4+4])
			}

			for _,c := range cmds {
				if c.code == INIT_DESTROYPOD {
					glog.Info("got pod finish message after having send destroy message")
					looping = false
					ctx.Hub <- &CommandAck {
						reply: c.code,
					}
					break
				}
			}

			glog.V(1).Infof("Pod finished, returned %d values", num)

			ctx.Hub <- &PodFinished{
				result: results,
			}
		} else {
			if glog.V(1) {
				glog.Infof("send command %d to init, payload: '%s'.", cmd.code, string(cmd.message))
			}
			init.Write(newVmMessage(cmd))
			cmds = append(cmds, cmd)
			if pongTimer == nil {
				glog.V(1).Info("message sent, set pong timer")
				pongTimer = time.AfterFunc(30*time.Second, func() {
					ctx.Hub <- &Interrupted{Reason: "init not reply ping mesg"}
				})
			}
		}
	}

	if pingTimer != nil {
		pingTimer.Stop()
	}
	if pongTimer != nil {
		pongTimer.Stop()
	}
}

func waitInitAck(ctx *VmContext, init *net.UnixConn) {
	for {
		res, err := readVmMessage(init)
		if err != nil {
			ctx.Hub <- &Interrupted{Reason: "init socket failed " + err.Error()}
			return
		} else if res.code == INIT_ACK || res.code == INIT_ERROR || res.code == INIT_FINISHPOD {
			ctx.vm <- res
		}
	}
}
