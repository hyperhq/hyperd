package hypervisor

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
)

type WindowSize struct {
	Row    uint16 `json:"row"`
	Column uint16 `json:"column"`
}

type TtyIO struct {
	Stdin  io.ReadCloser
	Stdout io.Writer
	Stderr io.Writer
}

type ttyAttachments struct {
	closed    bool
	stdioSeq  uint64
	stderrSeq uint64
	ttyio     *TtyIO
}

type pseudoTtys struct {
	attachId uint64 //next available attachId for attached tty
	channel  chan *hyperstartapi.TtyMessage
	ttys     map[uint64]*ttyAttachments
	lock     sync.Mutex
}

func newPts() *pseudoTtys {
	return &pseudoTtys{
		attachId: 1,
		channel:  make(chan *hyperstartapi.TtyMessage, 256),
		ttys:     make(map[uint64]*ttyAttachments),
	}
}

func readTtyMessage(conn *net.UnixConn) (*hyperstartapi.TtyMessage, error) {
	needRead := 12
	length := 0
	read := 0
	buf := make([]byte, 512)
	res := []byte{}
	for read < needRead {
		want := needRead - read
		if want > 512 {
			want = 512
		}
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read tty data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		if length == 0 && read >= 12 {
			length = int(binary.BigEndian.Uint32(res[8:12]))
			if length > 12 {
				needRead = length
			}
		}
	}

	return &hyperstartapi.TtyMessage{
		Session: binary.BigEndian.Uint64(res[:8]),
		Message: res[12:],
	}, nil
}

func waitTtyMessage(ctx *VmContext, conn *net.UnixConn) {
	for {
		msg, ok := <-ctx.ptys.channel
		if !ok {
			glog.V(1).Info("tty chan closed, quit sent goroutine")
			break
		}

		if _, ok := ctx.ptys.ttys[msg.Session]; ok {
			_, err := conn.Write(msg.ToBuffer())
			if err != nil {
				glog.V(1).Info("Cannot write to tty socket: ", err.Error())
				return
			}
		}
	}
}

func waitPts(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.TtySockName)
	if err != nil {
		glog.Error("Cannot connect to tty socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot connect to tty socket " + err.Error(),
		}
		return
	}

	glog.V(1).Info("tty socket connected")

	go waitTtyMessage(ctx, conn.(*net.UnixConn))

	for {
		res, err := readTtyMessage(conn.(*net.UnixConn))
		if err != nil {
			glog.V(1).Info("tty socket closed, quit the reading goroutine ", err.Error())
			ctx.Hub <- &Interrupted{Reason: "tty socket failed " + err.Error()}
			close(ctx.ptys.channel)
			return
		}
		glog.V(1).Infof("tty: read %d bytes for stream %d", len(res.Message), res.Session)
		if len(res.Message) == 0 {
			glog.V(1).Infof("session %d closed by peer, close pty", res.Session)
			if ctx.vmHyperstartAPIVersion > 4242 {
				ctx.ptys.Remove(res.Session)
			} else if ta, ok := ctx.ptys.ttys[res.Session]; ok {
				ta.closed = true
			} else {
				ctx.ptys.StdioConnect(res.Session, 0, nil)
			}
		} else if ta, ok := ctx.ptys.ttys[res.Session]; ok {
			if ta.closed {
				var code uint8 = 255
				if len(res.Message) == 1 {
					code = uint8(res.Message[0])
				}
				glog.V(1).Infof("session %d, exit code %d", res.Session, code)
				ctx.ptys.Remove4242(ctx, res.Session, code)
			} else {
				if ta.ttyio.Stdout != nil && res.Session == ta.stdioSeq {
					_, err := ta.ttyio.Stdout.Write(res.Message)
					if err != nil {
						glog.V(1).Infof("fail to write session %d, close stdio", res.Session)
						ctx.ptys.Remove(ta.stdioSeq)
					}
				}
				if ta.ttyio.Stderr != nil && res.Session == ta.stderrSeq {
					_, err := ta.ttyio.Stderr.Write(res.Message)
					if err != nil {
						glog.V(1).Infof("fail to write session %d, close stdio", res.Session)
						ctx.ptys.Remove(ta.stdioSeq)
					}
				}
			}
		}
	}
}

func (tty *TtyIO) Close() {
	glog.V(1).Info("Close tty ")

	if tty.Stdin != nil {
		tty.Stdin.Close()
	}
	cf := func(w io.Writer) {
		if w == nil {
			return
		}
		if c, ok := w.(io.WriteCloser); ok {
			c.Close()
		}
	}
	cf(tty.Stdout)
	cf(tty.Stderr)
}

func (pts *pseudoTtys) nextAttachId() uint64 {
	pts.lock.Lock()
	id := pts.attachId
	pts.attachId++
	pts.lock.Unlock()
	return id
}

func (pts *pseudoTtys) Remove4242(ctx *VmContext, session uint64, code uint8) {
	if ta, ok := pts.ttys[session]; ok {
		ack := make(chan bool, 1)
		kind := types.E_CONTAINER_FINISHED
		id := ctx.LookupBySession(session)

		if id == "" {
			if id = ctx.LookupExecBySession(session); id != "" {
				kind = types.E_EXEC_FINISHED
				//remove exec automatically
				ctx.DeleteExec(id)
			}
			ctx.Log(DEBUG, "found finished exec %s", id)
		}

		if id != "" {
			ctx.reportProcessFinished(kind, &types.ProcessFinished{
				Id: id, Code: code, Ack: ack,
			})
			ctx.Log(DEBUG, "report event %d (8:exec/9container) finish, id: %s", kind, id)
			// TODO: We should have a timeout here
			// wait for pod handler setting up exitcode for container
			select {
			case <-ack:
				ctx.Log(TRACE, "report event %s finish: done", id)
			case <-time.After(5 * time.Minute):
				ctx.Log(TRACE, "report event %s finish: timeout", id)
			}
		}

		pts.lock.Lock()
		ta.ttyio.Close()
		delete(pts.ttys, ta.stdioSeq)
		if ta.stderrSeq > 0 {
			delete(pts.ttys, ta.stderrSeq)
		}
		pts.lock.Unlock()
	}
}

func (pts *pseudoTtys) Remove(session uint64) {
	pts.lock.Lock()
	defer pts.lock.Unlock()
	if ta, ok := pts.ttys[session]; ok {
		ta.ttyio.Close()
		delete(pts.ttys, ta.stdioSeq)
		if ta.stderrSeq > 0 {
			delete(pts.ttys, ta.stderrSeq)
		}
	}
}

func (pts *pseudoTtys) StdioConnect(stdioSeq, stderrSeq uint64, tty *TtyIO) {
	pts.lock.Lock()
	if _, ok := pts.ttys[stdioSeq]; !ok {
		ta := &ttyAttachments{
			stdioSeq:  stdioSeq,
			stderrSeq: stderrSeq,
			ttyio:     tty,
		}
		pts.ttys[stdioSeq] = ta
		if stderrSeq > 0 {
			pts.ttys[stderrSeq] = ta
		}
	}
	pts.lock.Unlock()
}

func (pts *pseudoTtys) startStdin(session uint64) {
	pts.lock.Lock()
	defer pts.lock.Unlock()
	ta, ok := pts.ttys[session]
	if !ok {
		return
	}

	if ta.ttyio.Stdin != nil {
		go func() {
			buf := make([]byte, 32)

			defer func() { recover() }()
			for {
				nr, err := ta.ttyio.Stdin.Read(buf)
				if err != nil {
					glog.Info("a stdin closed, ", err.Error())
					if err == io.EOF {
						// send eof to hyperstart
						glog.V(1).Infof("session %d send eof to hyperstart", session)
						pts.channel <- &hyperstartapi.TtyMessage{
							Session: session,
							Message: make([]byte, 0),
						}
					} else if ta, ok := pts.ttys[session]; ok {
						pts.Remove(ta.stdioSeq)
					}
					return
				}

				glog.V(3).Infof("trying to input %d chars to stream %d", nr, session)

				mbuf := make([]byte, nr)
				copy(mbuf, buf[:nr])
				pts.channel <- &hyperstartapi.TtyMessage{
					Session: session,
					Message: mbuf[:nr],
				}
			}
		}()
	}
}

func (vm *Vm) Attach(tty *TtyIO, container string, size *WindowSize) error {
	cmd := &AttachCommand{
		Streams:   tty,
		Size:      size,
		Container: container,
	}

	return vm.GenericOperation("Attach", func(ctx *VmContext, result chan<- error) {
		ctx.attachCmd(cmd, result)
	}, StateRunning)
}
