package hypervisor

import (
	"encoding/binary"
	"hyper/lib/glog"
	"hyper/types"
	"io"
	"net"
	"sync"
)

type WindowSize struct {
	Row    uint16 `json:"row"`
	Column uint16 `json:"column"`
}

type TtyIO struct {
	Stdin     io.ReadCloser
	Stdout    io.WriteCloser
	ClientTag string
	Callback  chan *types.QemuResponse
}

type ttyAttachments struct {
	container   int
	persistent  bool
	attachments []*TtyIO
}

type pseudoTtys struct {
	channel chan *ttyMessage
	ttys    map[uint64]*ttyAttachments
	lock    *sync.Mutex
}

type ttyMessage struct {
	session uint64
	message []byte
}

func (tm *ttyMessage) toBuffer() []byte {
	length := len(tm.message) + 12
	buf := make([]byte, length)
	binary.BigEndian.PutUint64(buf[:8], tm.session)
	binary.BigEndian.PutUint32(buf[8:12], uint32(length))
	copy(buf[12:], tm.message)
	return buf
}

func newPts() *pseudoTtys {
	return &pseudoTtys{
		channel: make(chan *ttyMessage, 256),
		ttys:    make(map[uint64]*ttyAttachments),
		lock:    &sync.Mutex{},
	}
}

func readTtyMessage(conn *net.UnixConn) (*ttyMessage, error) {
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
		glog.V(1).Infof("tty: trying to read %d bytes", want)
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read tty data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		glog.V(1).Infof("tty: read %d/%d [length = %d]", read, needRead, length)

		if length == 0 && read >= 12 {
			length = int(binary.BigEndian.Uint32(res[8:12]))
			glog.V(1).Infof("data length is %d", length)
			if length > 12 {
				needRead = length
			}
		}
	}

	return &ttyMessage{
		session: binary.BigEndian.Uint64(res[:8]),
		message: res[12:],
	}, nil
}

func waitTtyMessage(ctx *VmContext, conn *net.UnixConn) {
	for {
		msg, ok := <-ctx.ptys.channel
		if !ok {
			glog.V(1).Info("tty chan closed, quit sent goroutine")
			break
		}

		glog.V(3).Infof("trying to write to session %d", msg.session)

		if _, ok := ctx.ptys.ttys[msg.session]; ok {
			_, err := conn.Write(msg.toBuffer())
			if err != nil {
				glog.V(1).Info("Cannot write to tty socket: ", err.Error())
				return
			}
		}
	}
}

func waitPts(ctx *VmContext) {
	conn, err := unixSocketConnect(ctx.ttySockName)
	if err != nil {
		glog.Error("Cannot connect to tty socket ", err.Error())
		ctx.hub <- &InitFailedEvent{
			reason: "Cannot connect to tty socket " + err.Error(),
		}
		return
	}

	glog.V(1).Info("tty socket connected")

	go waitTtyMessage(ctx, conn.(*net.UnixConn))

	for {
		res, err := readTtyMessage(conn.(*net.UnixConn))
		if err != nil {
			glog.V(1).Info("tty socket closed, quit the reading goroutine ", err.Error())
			ctx.hub <- &Interrupted{reason: "tty socket failed " + err.Error()}
			close(ctx.ptys.channel)
			return
		}
		if ta, ok := ctx.ptys.ttys[res.session]; ok {
			if len(res.message) == 0 {
				glog.V(1).Infof("session %d closed by peer, close pty", res.session)
				ctx.ptys.Close(ctx, res.session)
			} else {
				for _, tty := range ta.attachments {
					if tty.Stdout != nil {
						_, err := tty.Stdout.Write(res.message)
						if err != nil {
							glog.V(1).Infof("fail to write session %d, close pty attachment", res.session)
							ctx.ptys.Detach(ctx, res.session, tty)
						}
					}
				}
			}
		}
	}
}

func newAttachments(idx int, persist bool) *ttyAttachments {
	return &ttyAttachments{
		container:   idx,
		persistent:  persist,
		attachments: []*TtyIO{},
	}
}

func newAttachmentsWithTty(idx int, persist bool, tty *TtyIO) *ttyAttachments {
	return &ttyAttachments{
		container:   idx,
		persistent:  persist,
		attachments: []*TtyIO{tty},
	}
}

func (ta *ttyAttachments) attach(tty *TtyIO) {
	ta.attachments = append(ta.attachments, tty)
}

func (ta *ttyAttachments) detach(tty *TtyIO) {
	at := []*TtyIO{}
	detached := false
	for _, t := range ta.attachments {
		if tty.ClientTag != t.ClientTag {
			at = append(at, t)
		} else {
			detached = true
		}
	}
	if detached {
		ta.attachments = at
	}
}

func (ta *ttyAttachments) close() []string {
	tags := []string{}
	for _, t := range ta.attachments {
		tags = append(tags, t.Close())
	}
	ta.attachments = []*TtyIO{}
	return tags
}

func (ta *ttyAttachments) empty() bool {
	return len(ta.attachments) == 0
}

func (tty *TtyIO) Close() string {
	if tty.Stdin != nil {
		tty.Stdin.Close()
	}
	if tty.Stdout != nil {
		tty.Stdout.Close()
	}
	if tty.Callback != nil {
		tty.Callback <- &types.QemuResponse{
			Code:  types.E_EXEC_FINISH,
			Cause: "Command finished",
		}
	}
	return tty.ClientTag
}

func (pts *pseudoTtys) Detach(ctx *VmContext, session uint64, tty *TtyIO) {
	if ta, ok := ctx.ptys.ttys[session]; ok {
		ctx.ptys.lock.Lock()
		ta.detach(tty)
		ctx.ptys.lock.Unlock()
		if !ta.persistent && ta.empty() {
			ctx.ptys.Close(ctx, session)
		}
		ctx.clientDereg(tty.Close())
	}
}

func (pts *pseudoTtys) Close(ctx *VmContext, session uint64) {
	if ta, ok := pts.ttys[session]; ok {
		pts.lock.Lock()
		tags := ta.close()
		delete(pts.ttys, session)
		pts.lock.Unlock()
		for _, t := range tags {
			ctx.clientDereg(t)
		}
	}
}

func (pts *pseudoTtys) ptyConnect(ctx *VmContext, container int, session uint64, tty *TtyIO) {

	pts.lock.Lock()
	if ta, ok := pts.ttys[session]; ok {
		ta.attach(tty)
	} else {
		pts.ttys[session] = newAttachmentsWithTty(container, false, tty)
	}
	pts.lock.Unlock()

	if tty.Stdin != nil {
		go func() {
			buf := make([]byte, 32)
			defer pts.Detach(ctx, session, tty)
			defer func() { recover() }()
			for {
				nr, err := tty.Stdin.Read(buf)
				if err != nil {
					glog.Info("a stdin closed, ", err.Error())
					return
				} else if nr == 1 && buf[0] == ExitChar {
					glog.Info("got stdin detach char, exit term")
					return
				}

				glog.V(3).Infof("trying to input char: %d and %d chars", buf[0], nr)

				mbuf := make([]byte, nr)
				copy(mbuf, buf[:nr])
				pts.channel <- &ttyMessage{
					session: session,
					message: mbuf[:nr],
				}
			}
		}()
	}

	return
}

func LinerTty(output chan string) *TtyIO {
	r, w := io.Pipe()
	go ttyLiner(r, output)
	return &TtyIO{
		Stdin:    nil,
		Stdout:   w,
		Callback: nil,
	}
}

func ttyLiner(conn io.Reader, output chan string) {
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
