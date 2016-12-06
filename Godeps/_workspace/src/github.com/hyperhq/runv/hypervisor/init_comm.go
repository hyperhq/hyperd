package hypervisor

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/lib/telnet"
	"github.com/hyperhq/runv/lib/utils"
)

type hyperstartCmd struct {
	Code    uint32
	Message interface{}
	Event   VmEvent

	// result
	retMsg []byte
	result chan<- error
}

func defaultHyperstartResultChan(ctx *VmContext, cmd *hyperstartCmd) chan<- error {
	result := make(chan error, 1)
	go func() {
		err := <-result
		if err == nil {
			ctx.Hub <- &CommandAck{
				reply: cmd,
				msg:   cmd.retMsg,
			}
		} else {
			ctx.Hub <- &CommandError{
				reply: cmd,
				msg:   cmd.retMsg,
			}
		}
	}()
	return result
}

func waitConsoleOutput(ctx *VmContext) {

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

func NewVmMessage(m *hyperstartapi.DecodedMessage) []byte {
	length := len(m.Message) + 8
	msg := make([]byte, length)
	binary.BigEndian.PutUint32(msg[:], uint32(m.Code))
	binary.BigEndian.PutUint32(msg[4:], uint32(length))
	copy(msg[8:], m.Message)
	return msg
}

func ReadVmMessage(conn *net.UnixConn) (*hyperstartapi.DecodedMessage, error) {
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

	return &hyperstartapi.DecodedMessage{
		Code:    binary.BigEndian.Uint32(res[:4]),
		Message: res[8:],
	}, nil
}

func waitInitReady(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot connect to hyper socket " + err.Error(),
		}
		return
	}

	if ctx.Boot.BootFromTemplate {
		glog.Info("boot from template")
		ctx.PauseState = PauseStatePaused
		ctx.Hub <- &InitConnectedEvent{conn: conn.(*net.UnixConn)}
		go waitCmdToInit(ctx, conn.(*net.UnixConn))
		// TODO call getVMHyperstartAPIVersion(ctx) after unpaused
		return
	}

	glog.Info("Wating for init messages...")

	msg, err := ReadVmMessage(conn.(*net.UnixConn))
	if err != nil {
		glog.Error("read init message failed... ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "read init message failed... " + err.Error(),
		}
		conn.Close()
	} else if msg.Code == hyperstartapi.INIT_READY {
		glog.Info("Get init ready message")
		ctx.Hub <- &InitConnectedEvent{conn: conn.(*net.UnixConn)}
		go waitCmdToInit(ctx, conn.(*net.UnixConn))
		if !ctx.Boot.BootToBeTemplate {
			getVMHyperstartAPIVersion(ctx)
		}
	} else {
		glog.Warningf("Get init message %d", msg.Code)
		ctx.Hub <- &InitFailedEvent{
			Reason: fmt.Sprintf("Get init message %d", msg.Code),
		}
		conn.Close()
	}
}

func connectToInit(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot re-connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot re-connect to hyper socket " + err.Error(),
		}
		return
	}

	go waitCmdToInit(ctx, conn.(*net.UnixConn))
	getVMHyperstartAPIVersion(ctx)
}

func getVMHyperstartAPIVersion(ctx *VmContext) error {
	result := make(chan error, 1)
	vcmd := &hyperstartCmd{
		Code:   hyperstartapi.INIT_VERSION,
		result: result,
	}
	ctx.vm <- vcmd
	err := <-result
	if err != nil {
		glog.Infof("get hyperstart API version error: %v\n", err)
		return err
	}
	if len(vcmd.retMsg) < 4 {
		glog.Infof("get hyperstart API version error, wrong retMsg: %v\n", vcmd.retMsg)
		return fmt.Errorf("unexpected version string: %v\n", vcmd.retMsg)
	}
	ctx.vmHyperstartAPIVersion = binary.BigEndian.Uint32(vcmd.retMsg[:4])
	glog.Infof("hyperstart API version:%d, VM hyperstart API version: %d\n", hyperstartapi.VERSION, ctx.vmHyperstartAPIVersion)
	// TODO setup compatibility attributes here
	return nil
}

func waitCmdToInit(ctx *VmContext, init *net.UnixConn) {
	looping := true
	cmds := []*hyperstartCmd{}

	var data []byte
	var timeout bool = false
	var index int = 0
	var got int = 0
	var pingTimer *time.Timer = nil
	var pongTimer *time.Timer = nil

	go waitInitAck(ctx, init)

	for looping {
		cmd, ok := <-ctx.vm
		if !ok {
			glog.Info("vm channel closed, quit")
			break
		}
		if cmd.result == nil {
			cmd.result = defaultHyperstartResultChan(ctx, cmd)
		}
		glog.Infof("got cmd:%d", cmd.Code)
		if cmd.Code == hyperstartapi.INIT_ACK || cmd.Code == hyperstartapi.INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got response of shutdown command, last round of command to init")
					looping = false
				}
				if cmd.Code == hyperstartapi.INIT_ACK {
					if cmds[0].Code != hyperstartapi.INIT_PING {
						cmds[0].retMsg = cmd.retMsg
						cmds[0].result <- nil
					}
				} else {
					cmds[0].retMsg = cmd.retMsg
					cmds[0].result <- fmt.Errorf("Error: %s", string(cmd.retMsg))
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
						ctx.vm <- &hyperstartCmd{
							Code: hyperstartapi.INIT_PING,
						}
						pingTimer = nil
					})
				} else {
					pingTimer.Reset(30 * time.Second)
				}
			} else {
				glog.Error("got ack but no command in queue")
			}
		} else if cmd.Code == hyperstartapi.INIT_FINISHPOD {
			num := len(cmd.retMsg) / 4
			results := make([]uint32, num)
			for i := 0; i < num; i++ {
				results[i] = binary.BigEndian.Uint32(cmd.retMsg[i*4 : i*4+4])
			}

			for _, c := range cmds {
				if c.Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got pod finish message after having send destroy message")
					looping = false
					ctx.Hub <- &CommandAck{
						reply: c,
					}
					break
				}
			}

			glog.V(1).Infof("Pod finished, returned %d values", num)

			ctx.Hub <- &PodFinished{
				result: results,
			}
		} else {
			if cmd.Code == hyperstartapi.INIT_NEXT {
				glog.V(1).Infof("get command NEXT")

				got += int(binary.BigEndian.Uint32(cmd.retMsg[0:4]))
				glog.V(1).Infof("send %d, receive %d", index, got)
				timeout = false
				if index == got {
					/* received the sent out message */
					tmp := data[index:]
					data = tmp
					index = 0
					got = 0
				}
			} else {
				var message []byte
				if message1, ok := cmd.Message.([]byte); ok {
					message = message1
				} else if message2, err := json.Marshal(cmd.Message); err == nil {
					message = message2
				} else {
					glog.Infof("marshal command %d failed. object: %v", cmd.Code, cmd.Message)
					cmd.result <- fmt.Errorf("marshal command %d failed", cmd.Code)
					continue
				}

				msg := &hyperstartapi.DecodedMessage{
					Code:    cmd.Code,
					Message: message,
				}
				glog.V(1).Infof("send command %d to init, payload: '%s'.", cmd.Code, string(msg.Message))
				cmds = append(cmds, cmd)
				data = append(data, NewVmMessage(msg)...)
				timeout = true
			}

			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote, _ := init.Write(data[:end])
				glog.V(1).Infof("write %d to init, payload: '%s'.", wrote, data[:end])
				index += wrote
			}

			if timeout && pongTimer == nil {
				glog.V(1).Info("message sent, set pong timer")
				pongTimer = time.AfterFunc(30*time.Second, func() {
					if ctx.PauseState == PauseStateUnpaused {
						ctx.Hub <- &Interrupted{Reason: "init not reply ping mesg"}
					}
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
		res, err := ReadVmMessage(init)
		if err != nil {
			ctx.Hub <- &Interrupted{Reason: "init socket failed " + err.Error()}
			return
		} else if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR || res.Code == hyperstartapi.INIT_FINISHPOD {
			ctx.vm <- &hyperstartCmd{Code: res.Code, retMsg: res.Message}
		}
	}
}
