package libhyperstart

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/lib/utils"
)

type pKey struct{ c, p string }
type pState struct {
	stdioSeq   uint64
	stderrSeq  uint64
	stdinPipe  streamIn
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser
}

type jsonBasedHyperstart struct {
	sync.RWMutex
	vmAPIVersion       uint32
	closed             bool
	lastStreamSeq      uint64
	procs              map[pKey]*pState
	streamOuts         map[uint64]streamOut
	ctlChan            chan *hyperstartCmd
	streamChan         chan *hyperstartapi.TtyMessage
	processAsyncEvents chan hyperstartapi.ProcessAsyncEvent
}

type hyperstartCmd struct {
	Code    uint32
	Message interface{}

	// result
	retMsg []byte
	result chan<- error
}

func NewJsonBasedHyperstart(ctlSock, streamSock string, lastStreamSeq uint64, waitReady bool) Hyperstart {
	h := &jsonBasedHyperstart{
		procs:              make(map[pKey]*pState),
		lastStreamSeq:      lastStreamSeq,
		streamOuts:         make(map[uint64]streamOut),
		ctlChan:            make(chan *hyperstartCmd, 128),
		streamChan:         make(chan *hyperstartapi.TtyMessage, 128),
		processAsyncEvents: make(chan hyperstartapi.ProcessAsyncEvent, 16),
	}
	go handleStreamSock(h, streamSock)
	go handleCtlSock(h, ctlSock, waitReady)
	return h
}

func (h *jsonBasedHyperstart) Close() {
	h.Lock()
	defer h.Unlock()
	if !h.closed {
		glog.V(3).Info("close jsonBasedHyperstart")
		for pk := range h.procs {
			h.processAsyncEvents <- hyperstartapi.ProcessAsyncEvent{Container: pk.c, Process: pk.p, Event: "finished", Status: 255}
		}
		h.procs = make(map[pKey]*pState)
		for _, out := range h.streamOuts {
			out.Close()
		}
		h.streamOuts = make(map[uint64]streamOut)
		close(h.ctlChan)
		close(h.streamChan)
		close(h.processAsyncEvents)
		for cmd := range h.ctlChan {
			if cmd.Code != hyperstartapi.INIT_ACK && cmd.Code != hyperstartapi.INIT_ERROR {
				cmd.result <- fmt.Errorf("hyperstart closed")
			}
		}
		h.closed = true
	}
}

func (h *jsonBasedHyperstart) ProcessAsyncEvents() (<-chan hyperstartapi.ProcessAsyncEvent, error) {
	return h.processAsyncEvents, nil
}

func (h *jsonBasedHyperstart) LastStreamSeq() uint64 {
	return h.lastStreamSeq
}

func newVmMessage(m *hyperstartapi.DecodedMessage) []byte {
	length := len(m.Message) + 8
	msg := make([]byte, length)
	binary.BigEndian.PutUint32(msg[:], uint32(m.Code))
	binary.BigEndian.PutUint32(msg[4:], uint32(length))
	copy(msg[8:], m.Message)
	return msg
}

func readVmMessage(conn io.Reader) (*hyperstartapi.DecodedMessage, error) {
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
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read init data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		if length == 0 && read >= 8 {
			length = int(binary.BigEndian.Uint32(res[4:8]))
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

func handleCtlSock(h *jsonBasedHyperstart, ctlSock string, waitReady bool) error {
	conn, err := utils.SocketConnect(ctlSock)
	if err != nil {
		glog.Errorf("Cannot connect to ctl socket %s: %v", ctlSock, err)
		h.Close()
		return err
	}

	if waitReady {
		glog.V(3).Info("Wating for init messages...")
		msg, err := readVmMessage(conn)
		if err != nil {
			conn.Close()
			glog.Errorf("error when readVmMessage() for ready message: %v", err)
			h.Close()
			return err
		} else if msg.Code != hyperstartapi.INIT_READY {
			conn.Close()
			glog.Errorf("Expect INIT_READY, but get init message %d", msg.Code)
			h.Close()
			return fmt.Errorf("Expect INIT_READY, but get init message %d", msg.Code)
		}
	}

	go handleMsgToHyperstart(h, conn)
	go handleMsgFromHyperstart(h, conn)

	h.vmAPIVersion, err = h.APIVersion()
	glog.V(3).Infof("hyperstart API version:%d, VM hyperstart API version: %d\n", hyperstartapi.VERSION, h.vmAPIVersion)
	if err != nil {
		h.Close()
	}
	return err
}

func (h *jsonBasedHyperstart) hyperstartCommandWithRetMsg(code uint32, msg interface{}) (retMsg []byte, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("send ctl channel error, the hyperstart might have closed")
		}
	}()
	result := make(chan error, 1)
	vcmd := &hyperstartCmd{
		Code:    code,
		Message: msg,
		result:  result,
	}
	h.ctlChan <- vcmd
	err = <-result
	return vcmd.retMsg, err
}

func (h *jsonBasedHyperstart) hyperstartCommand(code uint32, msg interface{}) error {
	_, err := h.hyperstartCommandWithRetMsg(code, msg)
	return err
}

func handleMsgToHyperstart(h *jsonBasedHyperstart, conn io.WriteCloser) {
	looping := true
	cmds := []*hyperstartCmd{}

	var data []byte
	var index int = 0
	var got int = 0

	for looping {
		cmd, ok := <-h.ctlChan
		if !ok {
			glog.V(3).Info("vm channel closed, quit")
			break
		}
		glog.V(3).Infof("got cmd:%d", cmd.Code)
		if cmd.Code == hyperstartapi.INIT_ACK || cmd.Code == hyperstartapi.INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].Code == hyperstartapi.INIT_DESTROYPOD {
					glog.V(3).Info("got response of shutdown command, last round of command to init")
					looping = false
				}
				if cmd.Code == hyperstartapi.INIT_ACK {
					cmds[0].retMsg = cmd.retMsg
					cmds[0].result <- nil
				} else {
					cmds[0].retMsg = cmd.retMsg
					cmds[0].result <- fmt.Errorf("Error: %s", string(cmd.retMsg))
				}
				cmds = cmds[1:]
			} else {
				glog.Error("got ack but no command in queue")
			}
		} else {
			if cmd.Code == hyperstartapi.INIT_NEXT {
				got += int(binary.BigEndian.Uint32(cmd.retMsg[0:4]))
				glog.V(3).Infof("get command NEXT: send %d, receive %d", index, got)
				if index == got {
					/* received the sent out message */
					tmp := data[index:]
					data = tmp
					index = 0
					got = 0
				}
			} else {
				if h.vmAPIVersion == 0 && (cmd.Code == hyperstartapi.INIT_EXECCMD || cmd.Code == hyperstartapi.INIT_NEWCONTAINER) {
					// delay version-awared command
					glog.V(3).Infof("delay version-awared command :%d", cmd.Code)
					time.AfterFunc(2*time.Millisecond, func() {
						h.ctlChan <- cmd
					})
					continue
				}
				var message []byte
				if message1, ok := cmd.Message.([]byte); ok {
					message = message1
				} else if message2, err := json.Marshal(cmd.Message); err == nil {
					message = message2
				} else {
					glog.Errorf("marshal command %d failed. object: %v", cmd.Code, cmd.Message)
					cmd.result <- fmt.Errorf("marshal command %d failed", cmd.Code)
					continue
				}
				if h.vmAPIVersion <= 4242 {
					var msgMap map[string]interface{}
					var msgErr error
					if cmd.Code == hyperstartapi.INIT_EXECCMD || cmd.Code == hyperstartapi.INIT_NEWCONTAINER {
						if msgErr = json.Unmarshal(message, &msgMap); msgErr == nil {
							if p, ok := msgMap["process"].(map[string]interface{}); ok {
								delete(p, "id")
							}
						}
					}
					if msgErr == nil && len(msgMap) != 0 {
						message, msgErr = json.Marshal(msgMap)
					}
					if msgErr != nil {
						cmd.result <- fmt.Errorf("handle 4242 command %d failed", cmd.Code)
						continue
					}
				}

				msg := &hyperstartapi.DecodedMessage{
					Code:    cmd.Code,
					Message: message,
				}
				glog.V(3).Infof("send command %d to init, payload: '%s'.", cmd.Code, string(msg.Message))
				cmds = append(cmds, cmd)
				data = append(data, newVmMessage(msg)...)
			}

			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote, _ := conn.Write(data[:end])
				glog.V(3).Infof("write %d to hyperstart.", wrote)
				index += wrote
			}
		}
	}
	conn.Close()
	for _, cmd := range cmds {
		cmd.result <- fmt.Errorf("hyperstart closed")
	}
	h.Close()
}

func handleMsgFromHyperstart(h *jsonBasedHyperstart, conn io.Reader) {
	for {
		res, err := readVmMessage(conn)
		if err == nil {
			glog.V(3).Infof("readVmMessage code: %d, len: %d", res.Code, len(res.Message))
		}
		if err != nil {
			h.Close()
			return
		} else if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR {
			h.ctlChan <- &hyperstartCmd{Code: res.Code, retMsg: res.Message}
		} else if res.Code == hyperstartapi.INIT_PROCESSASYNCEVENT {
			var pae hyperstartapi.ProcessAsyncEvent
			glog.V(3).Info("ProcessAsyncEvent: %s", string(res.Message))
			if err := json.Unmarshal(res.Message, &pae); err != nil {
				glog.Error("read invalid ProcessAsyncEvent")
			} else {
				h.sendProcessAsyncEvent(pae)
			}
		}
	}
}

func readTtyMessage(conn io.Reader) (*hyperstartapi.TtyMessage, error) {
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

func handleStreamToHyperstart(h *jsonBasedHyperstart, conn io.WriteCloser) {
	for {
		msg, ok := <-h.streamChan
		if !ok {
			glog.V(3).Info("tty chan closed, quit sent goroutine")
			conn.Close()
			break
		}

		_, err := conn.Write(msg.ToBuffer())
		if err != nil {
			glog.Errorf("Cannot write to tty socket: %v", err)
			return
		}
	}
}

func handleStreamSock(h *jsonBasedHyperstart, streamSock string) error {
	conn, err := utils.SocketConnect(streamSock)
	if err != nil {
		glog.Errorf("Cannot connect to stream socket %s: %v", streamSock, err)
		h.Close()
		return err
	}
	glog.V(3).Info("stream socket connected")

	go handleStreamToHyperstart(h, conn)
	go handleStreamFromHyperstart(h, conn)

	return nil
}

func handleStreamFromHyperstart(h *jsonBasedHyperstart, conn io.Reader) {
	for {
		res, err := readTtyMessage(conn)
		if err != nil {
			glog.Errorf("tty socket closed, quit the reading goroutine: %v", err)
			h.Close()
			return
		}
		glog.V(3).Infof("tty: read %d bytes for stream %d", len(res.Message), res.Session)
		h.RLock()
		out, ok := h.streamOuts[res.Session]
		h.RUnlock()
		if ok {
			if len(res.Message) > 0 {
				_, err := out.Write(res.Message)
				if err != nil {
					glog.Errorf("fail to write session %d, close stdio: %v", res.Session, err)
					out.Close()
					h.removeStreamOut(res.Session)
				}
			} else {
				glog.V(3).Infof("session %d closed by peer, close pty", res.Session)
				out.Close()
				h.removeStreamOut(res.Session)
			}
		} else if h.vmAPIVersion <= 4242 {
			var code uint8 = 255
			if len(res.Message) == 1 {
				code = uint8(res.Message[0])
			}
			glog.V(3).Infof("session %d, exit code %d", res.Session, code)
			h.sendProcessAsyncEvent4242(res.Session, code)
		}
	}
}

func (h *jsonBasedHyperstart) sendProcessAsyncEvent(pae hyperstartapi.ProcessAsyncEvent) {
	h.Lock()
	defer h.Unlock()
	pk := pKey{c: pae.Container, p: pae.Process}
	if _, ok := h.procs[pk]; ok {
		delete(h.procs, pk)
		h.processAsyncEvents <- pae
	}
}

func (h *jsonBasedHyperstart) sendProcessAsyncEvent4242(stdioSeq uint64, code uint8) {
	h.Lock()
	defer h.Unlock()
	for pk, ps := range h.procs {
		if ps.stdioSeq == stdioSeq {
			delete(h.procs, pk)
			h.processAsyncEvents <- hyperstartapi.ProcessAsyncEvent{Container: pk.c, Process: pk.p, Event: "finished", Status: int(code)}
		}
	}
}

func (h *jsonBasedHyperstart) removeStreamOut(seq uint64) {
	h.Lock()
	defer h.Unlock()
	// simple version: delete(h.streamOuts, seq), but the serial-based hyperstart
	// doesn't send eof of the stderr back, so we also remove stderr seq here
	if out, ok := h.streamOuts[seq]; ok {
		delete(h.streamOuts, seq)
		if seq == out.ps.stdioSeq && out.ps.stderrSeq > 0 {
			delete(h.streamOuts, out.ps.stderrSeq)
		}
	}
}

type streamIn struct {
	streamSeq uint64
	h         *jsonBasedHyperstart
}

func (s *streamIn) Write(data []byte) (ret int, err error) {
	b := data
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("Write stream error, the hyperstart might have closed")
		}
	}()

	for len(b) > 0 {
		if s.h == nil {
			return len(data) - len(b), fmt.Errorf("closed")
		}
		nr := 16
		if len(b) < nr {
			nr = len(b)
		}
		glog.V(3).Infof("trying to input %d chars to stream %d", nr, s.streamSeq)
		mbuf := make([]byte, nr)
		copy(mbuf, b[:nr])
		s.h.streamChan <- &hyperstartapi.TtyMessage{
			Session: s.streamSeq,
			Message: mbuf,
		}
		b = b[nr:]
	}
	return len(data), nil
}

func (s *streamIn) Close() (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("Write stream error, the hyperstart might have closed")
		}
	}()

	// send eof to hyperstart
	glog.V(3).Infof("session %d send eof to hyperstart", s.streamSeq)
	s.h.streamChan <- &hyperstartapi.TtyMessage{
		Session: s.streamSeq,
		Message: make([]byte, 0),
	}
	s.h = nil
	return nil
}

// Todo: make it nonblock
type streamOut struct {
	io.WriteCloser
	ps *pState // required for removeStreamOut()
}

func (h *jsonBasedHyperstart) APIVersion() (uint32, error) {
	retMsg, err := h.hyperstartCommandWithRetMsg(hyperstartapi.INIT_VERSION, nil)
	if err != nil {
		glog.Errorf("get hyperstart API version error: %v", err)
		return 0, err
	}
	if len(retMsg) < 4 {
		glog.Errorf("get hyperstart API version error, wrong retMsg: %v\n", retMsg)
		return 0, fmt.Errorf("unexpected version string: %v\n", retMsg)
	}
	return binary.BigEndian.Uint32(retMsg[:4]), nil
}

func (h *jsonBasedHyperstart) WriteFile(container, path string, data []byte) error {
	writeCmd, _ := json.Marshal(hyperstartapi.FileCommand{
		Container: container,
		File:      path,
	})
	writeCmd = append(writeCmd, data...)
	return h.hyperstartCommand(hyperstartapi.INIT_WRITEFILE, writeCmd)
}

func (h *jsonBasedHyperstart) ReadFile(container, path string) ([]byte, error) {
	return h.hyperstartCommandWithRetMsg(hyperstartapi.INIT_READFILE, &hyperstartapi.FileCommand{
		Container: container,
		File:      path,
	})
}

func (h *jsonBasedHyperstart) AddRoute(r []hyperstartapi.Route) error {
	return h.hyperstartCommand(hyperstartapi.INIT_SETUPROUTE, hyperstartapi.Routes{Routes: r})
}

func (h *jsonBasedHyperstart) UpdateInterface(dev, ip, mask string) error {
	return h.hyperstartCommand(hyperstartapi.INIT_SETUPINTERFACE, hyperstartapi.NetworkInf{
		Device:    dev,
		IpAddress: ip,
		NetMask:   mask,
	})
}

func (h *jsonBasedHyperstart) TtyWinResize4242(container, process string, row, col uint16) error {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		glog.Errorf("cannot find process: %s, %s", container, process)
		return fmt.Errorf("cannot find process: %s, %s", container, process)
	}

	cmd := map[string]interface{}{
		"seq":    p.stdioSeq,
		"row":    row,
		"column": col,
	}

	return h.hyperstartCommand(hyperstartapi.INIT_WINSIZE, cmd)
}

func (h *jsonBasedHyperstart) TtyWinResize(container, process string, row, col uint16) error {
	if h.vmAPIVersion <= 4242 {
		return h.TtyWinResize4242(container, process, row, col)
	}
	cmd := hyperstartapi.WindowSizeMessage{
		Container: container,
		Process:   process,
		Row:       row,
		Column:    col,
	}
	return h.hyperstartCommand(hyperstartapi.INIT_WINSIZE, cmd)
}

func (h *jsonBasedHyperstart) OnlineCpuMem() error {
	return h.hyperstartCommand(hyperstartapi.INIT_ONLINECPUMEM, nil)
}

func (h *jsonBasedHyperstart) allocStreamSeq() uint64 {
	seq := h.lastStreamSeq
	h.lastStreamSeq++
	return seq
}

func (h *jsonBasedHyperstart) setupProcessIo(ps *pState, terminal bool) {
	if ps.stdioSeq == 0 {
		ps.stdioSeq = h.allocStreamSeq()
	}
	stdoutPipe, stdout := io.Pipe() // TODO: make StreamOut nonblockable
	ps.stdoutPipe = stdoutPipe
	h.streamOuts[ps.stdioSeq] = streamOut{WriteCloser: stdout, ps: ps}
	if !terminal {
		if ps.stderrSeq == 0 {
			ps.stderrSeq = h.allocStreamSeq()
		}
		stderrPipe, stderr := io.Pipe()
		ps.stderrPipe = stderrPipe
		h.streamOuts[ps.stderrSeq] = streamOut{WriteCloser: stderr, ps: ps}
	}
	ps.stdinPipe = streamIn{streamSeq: ps.stdioSeq, h: h}
}

func (h *jsonBasedHyperstart) removeProcess(container, process string) {
	h.Lock()
	defer h.Unlock()
	pk := pKey{c: container, p: process}
	if ps, ok := h.procs[pk]; ok {
		delete(h.procs, pk)
		if ps.stdioSeq > 0 {
			delete(h.streamOuts, ps.stdioSeq)
		}
		if ps.stderrSeq > 0 {
			delete(h.streamOuts, ps.stderrSeq)
		}
	}
}

func (h *jsonBasedHyperstart) NewContainer(c *hyperstartapi.Container) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	h.Lock()
	if _, existed := h.procs[pKey{c: c.Id, p: c.Process.Id}]; existed {
		h.Unlock()
		return nil, nil, nil, fmt.Errorf("process id conflicts, the process of the id %s already exists", c.Process.Id)
	}
	ps := &pState{}
	h.setupProcessIo(ps, c.Process.Terminal)
	h.procs[pKey{c: c.Id, p: c.Process.Id}] = ps
	h.Unlock()

	c.Process.Stdio = ps.stdioSeq
	c.Process.Stderr = ps.stderrSeq

	err := h.hyperstartCommand(hyperstartapi.INIT_NEWCONTAINER, c)
	if err != nil {
		h.removeProcess(c.Id, c.Process.Id)
		return nil, nil, nil, err
	}
	return &ps.stdinPipe, ps.stdoutPipe, ps.stderrPipe, err
}

func (h *jsonBasedHyperstart) RestoreContainer(c *hyperstartapi.Container) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	h.Lock()
	if _, existed := h.procs[pKey{c: c.Id, p: c.Process.Id}]; existed {
		h.Unlock()
		return nil, nil, nil, fmt.Errorf("process id conflicts, the process of the id %s already exists", c.Process.Id)
	}
	h.Unlock()
	// Send SIGCONT signal to init to test whether it's alive.
	err := h.SignalProcess(c.Id, "init", syscall.SIGCONT)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("container not exist or already stopped: %v", err)
	}
	// restore procs/streamOuts map
	ps := &pState{
		stdioSeq:  c.Process.Stdio,
		stderrSeq: c.Process.Stderr,
	}
	h.Lock()
	h.setupProcessIo(ps, c.Process.Terminal)
	h.procs[pKey{c: c.Id, p: c.Process.Id}] = ps
	h.Unlock()

	return &ps.stdinPipe, ps.stdoutPipe, ps.stderrPipe, nil
}

func (h *jsonBasedHyperstart) AddProcess(container string, p *hyperstartapi.Process) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	h.Lock()
	if _, existed := h.procs[pKey{c: container, p: p.Id}]; existed {
		h.Unlock()
		return nil, nil, nil, fmt.Errorf("process id conflicts, the process of the id %s already exists", p.Id)
	}
	ps := &pState{}
	h.setupProcessIo(ps, p.Terminal)
	h.procs[pKey{c: container, p: p.Id}] = ps
	h.Unlock()

	p.Stdio = ps.stdioSeq
	p.Stderr = ps.stderrSeq
	err := h.hyperstartCommand(hyperstartapi.INIT_EXECCMD, hyperstartapi.ExecCommand{
		Container: container,
		Process:   *p,
	})
	if err != nil {
		h.removeProcess(container, p.Id)
		return nil, nil, nil, err
	}
	return &ps.stdinPipe, ps.stdoutPipe, ps.stderrPipe, err
}

func (h *jsonBasedHyperstart) SignalProcess(container, process string, signal syscall.Signal) error {
	if h.vmAPIVersion <= 4242 {
		if process == "init" {
			return h.hyperstartCommand(hyperstartapi.INIT_KILLCONTAINER, hyperstartapi.KillCommand{
				Container: container,
				Signal:    signal,
			})
		}
		return fmt.Errorf("only the init process of the container can be signaled")
	}
	return h.hyperstartCommand(hyperstartapi.INIT_SIGNALPROCESS, hyperstartapi.SignalCommand{
		Container: container,
		Process:   process,
		Signal:    signal,
	})
}

func (h *jsonBasedHyperstart) StartSandbox(pod *hyperstartapi.Pod) error {
	return h.hyperstartCommand(hyperstartapi.INIT_STARTPOD, pod)
}

func (h *jsonBasedHyperstart) DestroySandbox() error {
	return h.hyperstartCommand(hyperstartapi.INIT_DESTROYPOD, nil)
}
