package libhyperstart

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/lib/utils"
)

const (
	ERROR   = hlog.ERROR
	WARNING = hlog.WARNING
	INFO    = hlog.INFO
	DEBUG   = hlog.DEBUG
	TRACE   = hlog.TRACE
	EXTRA   = hlog.EXTRA
)

type pKey struct{ c, p string }
type pState struct {
	stdioSeq   uint64
	stderrSeq  uint64
	stdinPipe  streamIn
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser
	exitStatus *int
	outClosed  bool
	waitChan   chan int
}

type jsonBasedHyperstart struct {
	sync.RWMutex
	logPrefix     string
	vmAPIVersion  uint32
	closed        bool
	lastStreamSeq uint64
	procs         map[pKey]*pState
	streamOuts    map[uint64]streamOut
	ctlChan       chan *hyperstartCmd
	streamChan    chan *hyperstartapi.TtyMessage
}

// hyperstartPauseSync and hyperstartUnpause are private here and large enough
const hyperstartPauseSync = 1 << 31
const hyperstartUnpause = hyperstartPauseSync + 1

type hyperstartCmd struct {
	Code    uint32
	Message interface{}

	// result
	retMsg []byte
	result chan<- error
}

func NewJsonBasedHyperstart(id, ctlSock, streamSock string, lastStreamSeq uint64, waitReady, paused bool) (Hyperstart, error) {
	h := &jsonBasedHyperstart{
		logPrefix:     fmt.Sprintf("SB[%s] ", id),
		procs:         make(map[pKey]*pState),
		lastStreamSeq: lastStreamSeq,
		streamOuts:    make(map[uint64]streamOut),
		ctlChan:       make(chan *hyperstartCmd, 128),
		streamChan:    make(chan *hyperstartapi.TtyMessage, 128),
	}
	go handleStreamSock(h, streamSock)
	go handleCtlSock(h, ctlSock, waitReady, paused)
	return h, nil
}

func (h *jsonBasedHyperstart) LogLevel(level hlog.LogLevel) bool {
	return hlog.IsLogLevel(level)
}

func (h *jsonBasedHyperstart) LogPrefix() string {
	return h.logPrefix
}

func (h *jsonBasedHyperstart) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, h, 1, args...)
}

func (h *jsonBasedHyperstart) Close() {
	h.Lock()
	defer h.Unlock()
	if !h.closed {
		h.Log(TRACE, "close jsonBasedHyperstart")
		for _, out := range h.streamOuts {
			out.Close()
		}
		h.streamOuts = make(map[uint64]streamOut)
		for pk, ps := range h.procs {
			ps.outClosed = true
			ps.exitStatus = makeExitStatus(255)
			h.handleWaitProcess(pk, ps)
		}
		h.procs = make(map[pKey]*pState)
		close(h.ctlChan)
		close(h.streamChan)
		for cmd := range h.ctlChan {
			if cmd.Code != hyperstartapi.INIT_ACK && cmd.Code != hyperstartapi.INIT_ERROR {
				cmd.result <- fmt.Errorf("hyperstart closed")
			}
		}
		h.closed = true
	}
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
			hlog.Log(ERROR, "read init data failed")
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

func handleCtlSock(h *jsonBasedHyperstart, ctlSock string, waitReady, paused bool) error {
	conn, err := utils.SocketConnect(ctlSock)
	if err != nil {
		h.Log(ERROR, "Cannot connect to ctl socket %s: %v", ctlSock, err)
		h.Close()
		return err
	}

	if waitReady {
		h.Log(TRACE, "Wating for init messages...")
		msg, err := readVmMessage(conn)
		if err != nil {
			conn.Close()
			h.Log(ERROR, "error when readVmMessage() for ready message: %v", err)
			h.Close()
			return err
		} else if msg.Code != hyperstartapi.INIT_READY {
			conn.Close()
			h.Log(ERROR, "Expect INIT_READY, but get init message %d", msg.Code)
			h.Close()
			return fmt.Errorf("Expect INIT_READY, but get init message %d", msg.Code)
		}
	}

	go handleMsgToHyperstart(h, conn, paused)
	go handleMsgFromHyperstart(h, conn)

	if paused {
		return nil
	}

	h.vmAPIVersion, err = h.APIVersion()
	h.Log(TRACE, "hyperstart API version:%d, VM hyperstart API version: %d\n", hyperstartapi.VERSION, h.vmAPIVersion)
	if err != nil {
		h.Close()
	}
	return err
}

func (h *jsonBasedHyperstart) hyperstartCommandWithRetMsg(code uint32, msg interface{}) (retMsg []byte, err error) {
	if h.vmAPIVersion == 0 && (code == hyperstartapi.INIT_EXECCMD || code == hyperstartapi.INIT_NEWCONTAINER) {
		// delay version-awared command
		var t int64 = 2
		for h.vmAPIVersion == 0 {
			h.Log(TRACE, "delay version-awared command :%d by %dms", code)
			time.Sleep(time.Duration(t) * time.Millisecond)
			if t < 512 {
				t = t * 2
			}
		}
	}

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

func handleMsgToHyperstart(h *jsonBasedHyperstart, conn io.WriteCloser, paused bool) {
	looping := true
	cmds := []*hyperstartCmd{}

	var data []byte
	var index int = 0
	var got int = 0
	var pausing *hyperstartCmd

	for looping {
		cmd, ok := <-h.ctlChan
		if !ok {
			h.Log(TRACE, "vm channel closed, quit")
			break
		}
		h.Log(TRACE, "got cmd:%d", cmd.Code)
		if cmd.Code == hyperstartapi.INIT_ACK || cmd.Code == hyperstartapi.INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].Code == hyperstartapi.INIT_DESTROYPOD {
					h.Log(TRACE, "got response of shutdown command, last round of command to init")
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
				if pausing != nil && len(cmds) == 0 {
					pausing.result <- nil
					pausing = nil
				}
			} else {
				h.Log(ERROR, "got ack but no command in queue")
			}
		} else {
			if cmd.Code == hyperstartapi.INIT_NEXT {
				got += int(binary.BigEndian.Uint32(cmd.retMsg[0:4]))
				h.Log(TRACE, "get command NEXT: send %d, receive %d", index, got)
				if index == got {
					/* received the sent out message */
					tmp := data[index:]
					data = tmp
					index = 0
					got = 0
				}
			} else if cmd.Code == hyperstartPauseSync {
				paused = true
				pausing = cmd
				if len(cmds) == 0 {
					pausing.result <- nil
					pausing = nil
				}
			} else if cmd.Code == hyperstartUnpause {
				paused = false
				if pausing != nil {
					pausing.result <- nil
					pausing = nil
				}
				cmd.result <- nil
			} else {
				if paused {
					cmd.result <- fmt.Errorf("the vm is pausing/paused")
					continue
				}
				var message []byte
				if message1, ok := cmd.Message.([]byte); ok {
					message = message1
				} else if message2, err := json.Marshal(cmd.Message); err == nil {
					message = message2
				} else {
					h.Log(ERROR, "marshal command %d failed. object: %v", cmd.Code, cmd.Message)
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
				h.Log(TRACE, "send command %d to init, payload: '%s'.", cmd.Code, string(msg.Message))
				cmds = append(cmds, cmd)
				data = append(data, newVmMessage(msg)...)
			}

			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote, _ := conn.Write(data[:end])
				h.Log(TRACE, "write %d to hyperstart.", wrote)
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
	defer func() {
		if err := recover(); err != nil {
			h.Log(WARNING, "panic during handleMsgFromHyperstart (closed: %v): %v", h.closed, err)
		}
	}()
	for {
		res, err := readVmMessage(conn)
		if err == nil {
			h.Log(TRACE, "readVmMessage code: %d, len: %d", res.Code, len(res.Message))
		}
		if err != nil {
			h.Close()
			return
		} else if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR {
			h.ctlChan <- &hyperstartCmd{Code: res.Code, retMsg: res.Message}
		} else if res.Code == hyperstartapi.INIT_PROCESSASYNCEVENT {
			var pae hyperstartapi.ProcessAsyncEvent
			h.Log(TRACE, "ProcessAsyncEvent: %s", string(res.Message))
			if err := json.Unmarshal(res.Message, &pae); err != nil {
				h.Log(ERROR, "read invalid ProcessAsyncEvent")
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
			hlog.Log(ERROR, "read tty data failed")
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
			h.Log(TRACE, "tty chan closed, quit sent goroutine")
			conn.Close()
			break
		}

		_, err := conn.Write(msg.ToBuffer())
		if err != nil {
			h.Log(ERROR, "Cannot write to tty socket: %v", err)
			return
		}
	}
}

func handleStreamSock(h *jsonBasedHyperstart, streamSock string) error {
	conn, err := utils.SocketConnect(streamSock)
	if err != nil {
		h.Log(ERROR, "Cannot connect to stream socket %s: %v", streamSock, err)
		h.Close()
		return err
	}
	h.Log(TRACE, "stream socket connected")

	go handleStreamToHyperstart(h, conn)
	go handleStreamFromHyperstart(h, conn)

	return nil
}

func handleStreamFromHyperstart(h *jsonBasedHyperstart, conn io.Reader) {
	for {
		res, err := readTtyMessage(conn)
		if err != nil {
			h.Log(ERROR, "tty socket closed, quit the reading goroutine: %v", err)
			h.Close()
			return
		}
		h.Log(TRACE, "tty: read %d bytes for stream %d", len(res.Message), res.Session)
		h.RLock()
		out, ok := h.streamOuts[res.Session]
		h.RUnlock()
		if ok {
			if len(res.Message) > 0 {
				_, err := out.Write(res.Message)
				if err != nil {
					h.Log(ERROR, "fail to write session %d, close stdio: %v", res.Session, err)
					out.Close()
					h.removeStreamOut(res.Session)
				}
			} else {
				h.Log(TRACE, "session %d closed by peer, close pty", res.Session)
				out.Close()
				h.removeStreamOut(res.Session)
			}
		} else if h.vmAPIVersion <= 4242 {
			var code uint8 = 255
			if len(res.Message) == 1 {
				code = uint8(res.Message[0])
			}
			h.Log(TRACE, "session %d, exit code %d", res.Session, code)
			h.sendProcessAsyncEvent4242(res.Session, code)
		}
	}
}

func makeExitStatus(status int) *int { return &status }

func (h *jsonBasedHyperstart) handleWaitProcess(pk pKey, ps *pState) {
	if ps.exitStatus != nil && ps.waitChan != nil && ps.outClosed {
		delete(h.procs, pk)
		ps.waitChan <- *ps.exitStatus
	}
}

func (h *jsonBasedHyperstart) sendProcessAsyncEvent(pae hyperstartapi.ProcessAsyncEvent) {
	h.Lock()
	defer h.Unlock()
	pk := pKey{c: pae.Container, p: pae.Process}
	if ps, ok := h.procs[pk]; ok {
		ps.exitStatus = makeExitStatus(pae.Status)
		h.handleWaitProcess(pk, ps)
	}
}

func (h *jsonBasedHyperstart) sendProcessAsyncEvent4242(stdioSeq uint64, code uint8) {
	h.Lock()
	defer h.Unlock()
	for pk, ps := range h.procs {
		if ps.stdioSeq == stdioSeq {
			ps.exitStatus = makeExitStatus(int(code))
			h.handleWaitProcess(pk, ps)
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
		if seq == out.ps.stdioSeq {
			if out.ps.stderrSeq > 0 {
				h.streamOuts[out.ps.stderrSeq].Close()
				delete(h.streamOuts, out.ps.stderrSeq)
			}
			for pk, ps := range h.procs {
				if ps.stdioSeq == seq {
					ps.outClosed = true
					h.handleWaitProcess(pk, ps)
				}
			}
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
		s.h.Log(TRACE, "trying to input %d chars to stream %d", nr, s.streamSeq)
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
	s.h.Log(TRACE, "session %d send eof to hyperstart", s.streamSeq)
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
		h.Log(ERROR, "get hyperstart API version error: %v", err)
		return 0, err
	}
	if len(retMsg) < 4 {
		h.Log(ERROR, "get hyperstart API version error, wrong retMsg: %v\n", retMsg)
		return 0, fmt.Errorf("unexpected version string: %v\n", retMsg)
	}
	return binary.BigEndian.Uint32(retMsg[:4]), nil
}

func (h *jsonBasedHyperstart) PauseSync() error {
	return h.hyperstartCommand(hyperstartPauseSync, nil)
}

func (h *jsonBasedHyperstart) Unpause() error {
	err := h.hyperstartCommand(hyperstartUnpause, nil)

	if h.vmAPIVersion == 0 && err == nil {
		h.vmAPIVersion, err = h.APIVersion()
		h.Log(TRACE, "hyperstart API version:%d, VM hyperstart API version: %d\n", hyperstartapi.VERSION, h.vmAPIVersion)
		if err != nil {
			h.Close()
		}
	}

	return err
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

func (h *jsonBasedHyperstart) UpdateInterface(t InfUpdateType, dev, newName string, ipAddresses []hyperstartapi.IpAddress, mtu uint64) error {
	inf := hyperstartapi.NetworkInf{
		Device:      dev,
		IpAddresses: []hyperstartapi.IpAddress{},
	}
	switch t {
	case AddInf:
		inf.NewName = newName
		inf.Mtu = mtu
		inf.IpAddresses = ipAddresses
		if err := h.hyperstartCommand(hyperstartapi.INIT_SETUPINTERFACE, inf); err != nil {
			return fmt.Errorf("json: failed to send <add interface> command to hyperstart: %v", err)
		}
	case DelInf:
		if err := h.hyperstartCommand(hyperstartapi.INIT_DELETEINTERFACE, inf); err != nil {
			return fmt.Errorf("json: failed to send <delete interface> command to hyperstart. inf: %#v, error: %v", inf, err)
		}
	case AddIP:
		inf.IpAddresses = ipAddresses
		if err := h.hyperstartCommand(hyperstartapi.INIT_SETUPINTERFACE, inf); err != nil {
			return fmt.Errorf("json: failed to send <add ip> command to hyperstart: %v", err)
		}
	case DelIP:
		// TODO: add new interface to handle hyperstart delete interface @weizhang555
	case SetMtu:
		if mtu > 0 {
			inf.Mtu = mtu
			if err := h.hyperstartCommand(hyperstartapi.INIT_SETUPINTERFACE, inf); err != nil {
				return fmt.Errorf("json: failed to send <SetMtu> command to hyperstart: %v", err)
			}
		}
	}
	return nil
}

func (h *jsonBasedHyperstart) WriteStdin(container, process string, data []byte) (int, error) {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		h.Log(ERROR, "cannot find process: %s, %s", container, process)
		return 0, fmt.Errorf("cannot find process: %s, %s", container, process)
	}
	return p.stdinPipe.Write(data)
}

func (h *jsonBasedHyperstart) ReadStdout(container, process string, data []byte) (int, error) {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		h.Log(ERROR, "cannot find process: %s, %s", container, process)
		return 0, fmt.Errorf("cannot find process: %s, %s", container, process)
	}
	return p.stdoutPipe.Read(data)
}

func (h *jsonBasedHyperstart) ReadStderr(container, process string, data []byte) (int, error) {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		h.Log(ERROR, "cannot find process: %s, %s", container, process)
		return 0, fmt.Errorf("cannot find process: %s, %s", container, process)
	}
	if p.stderrSeq == 0 {
		return 0, io.EOF
	}
	return p.stderrPipe.Read(data)
}

func (h *jsonBasedHyperstart) CloseStdin(container, process string) error {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		h.Log(ERROR, "cannot find process: %s, %s", container, process)
		return fmt.Errorf("cannot find process: %s, %s", container, process)
	}
	return p.stdinPipe.Close()
}

func (h *jsonBasedHyperstart) TtyWinResize4242(container, process string, row, col uint16) error {
	h.RLock()
	p, ok := h.procs[pKey{c: container, p: process}]
	h.RUnlock()
	if !ok {
		h.Log(ERROR, "cannot find process: %s, %s", container, process)
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
	outPipe := utils.NewBytesPipe()
	ps.stdoutPipe = outPipe
	h.streamOuts[ps.stdioSeq] = streamOut{WriteCloser: outPipe, ps: ps}
	if !terminal {
		if ps.stderrSeq == 0 {
			ps.stderrSeq = h.allocStreamSeq()
		}
		errPipe := utils.NewBytesPipe()
		ps.stderrPipe = errPipe
		h.streamOuts[ps.stderrSeq] = streamOut{WriteCloser: errPipe, ps: ps}
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

func (h *jsonBasedHyperstart) NewContainer(c *hyperstartapi.Container) error {
	h.Lock()
	if _, existed := h.procs[pKey{c: c.Id, p: c.Process.Id}]; existed {
		h.Unlock()
		return fmt.Errorf("process id conflicts, the process of the id %s already exists", c.Process.Id)
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
		return err
	}
	return err
}

func (h *jsonBasedHyperstart) RestoreContainer(c *hyperstartapi.Container) error {
	h.Lock()
	if _, existed := h.procs[pKey{c: c.Id, p: c.Process.Id}]; existed {
		h.Unlock()
		return fmt.Errorf("process id conflicts, the process of the id %s already exists", c.Process.Id)
	}
	h.Unlock()
	// Send SIGCONT signal to init to test whether it's alive.
	err := h.SignalProcess(c.Id, "init", syscall.SIGCONT)
	if err != nil {
		return fmt.Errorf("container not exist or already stopped: %v", err)
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

	return nil
}

func (h *jsonBasedHyperstart) AddProcess(container string, p *hyperstartapi.Process) error {
	h.Lock()
	if _, existed := h.procs[pKey{c: container, p: p.Id}]; existed {
		h.Unlock()
		return fmt.Errorf("process id conflicts, the process of the id %s already exists", p.Id)
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
		return err
	}
	return err
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

// wait the process until exit. like waitpid()
// the state is saved until someone calls WaitProcess() if the process exited earlier
// the non-first call of WaitProcess() after process started MAY fail to find the process if the process exited earlier
func (h *jsonBasedHyperstart) WaitProcess(container, process string) int {
	h.Lock()
	pk := pKey{c: container, p: process}
	if ps, ok := h.procs[pk]; ok {
		if ps.waitChan == nil {
			ps.waitChan = make(chan int, 1)
			h.handleWaitProcess(pk, ps)
		}
		h.Unlock()
		status := <-ps.waitChan
		ps.waitChan <- status
		return status
	}
	h.Unlock()
	return -1
}

func (h *jsonBasedHyperstart) StartSandbox(pod *hyperstartapi.Pod) error {
	return h.hyperstartCommand(hyperstartapi.INIT_STARTPOD, pod)
}

func (h *jsonBasedHyperstart) DestroySandbox() error {
	return h.hyperstartCommand(hyperstartapi.INIT_DESTROYPOD, nil)
}
