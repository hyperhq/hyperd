package qemu

import (
	"encoding/json"
	"hyper/lib/glog"
	"io"
	"net"
	"syscall"
	"time"
)

type QmpInteraction interface {
	MessageType() int
}

type QmpQuit struct{}

type QmpTimeout struct{}

type QmpInit struct {
	decoder *json.Decoder
	conn    *net.UnixConn
}

type QmpInternalError struct{ cause string }

type QmpSession struct {
	commands []*QmpCommand
	callback QemuEvent
}

type QmpFinish struct {
	success  bool
	reason   map[string]interface{}
	callback QemuEvent
}

type QmpCommand struct {
	Execute   string                 `json:"execute"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Scm       []byte                 `json:"-"`
}

type QmpResponse struct {
	msg QmpInteraction
}

type QmpError struct {
	Cause map[string]interface{} `json:"error"`
}

type QmpResult struct {
	Return map[string]interface{} `json:"return"`
}

type QmpTimeStamp struct {
	Seconds      uint64 `json:"seconds"`
	Microseconds uint64 `json:"microseconds"`
}

type QmpEvent struct {
	Type      string       `json:"event"`
	Timestamp QmpTimeStamp `json:"timestamp"`
	Data      interface{}  `json:"data,omitempty"`
}

func (qmp *QmpInit) MessageType() int          { return QMP_INIT }
func (qmp *QmpQuit) MessageType() int          { return QMP_QUIT }
func (qmp *QmpTimeout) MessageType() int       { return QMP_TIMEOUT }
func (qmp *QmpInternalError) MessageType() int { return QMP_INTERNAL_ERROR }
func (qmp *QmpSession) MessageType() int       { return QMP_SESSION }
func (qmp *QmpSession) Finish() *QmpFinish {
	return &QmpFinish{
		success:  true,
		callback: qmp.callback,
	}
}
func (qmp *QmpFinish) MessageType() int { return QMP_FINISH }

func (qmp *QmpResult) MessageType() int { return QMP_RESULT }

func (qmp *QmpError) MessageType() int { return QMP_ERROR }
func (qmp *QmpError) Finish(callback QemuEvent) *QmpFinish {
	return &QmpFinish{
		success:  false,
		reason:   qmp.Cause,
		callback: callback,
	}
}

func (qmp *QmpEvent) MessageType() int { return QMP_EVENT }
func (qmp *QmpEvent) Event() int       { return EVENT_QMP_EVENT }
func (qmp *QmpEvent) timestamp() uint64 {
	return qmp.Timestamp.Microseconds + qmp.Timestamp.Seconds*1000000
}

func (qmp *QmpResponse) UnmarshalJSON(raw []byte) error {
	var tmp map[string]interface{}
	var err error = nil
	json.Unmarshal(raw, &tmp)
	glog.V(2).Info("got a message ", string(raw))
	if _, ok := tmp["event"]; ok {
		msg := &QmpEvent{}
		err = json.Unmarshal(raw, msg)
		glog.V(2).Info("got event: ", msg.Type)
		qmp.msg = msg
	} else if r, ok := tmp["return"]; ok {
		msg := &QmpResult{}
		switch r.(type) {
		case string:
			msg.Return = map[string]interface{}{
				"return": r.(string),
			}
		default:
			err = json.Unmarshal(raw, msg)
		}
		qmp.msg = msg
	} else if _, ok := tmp["error"]; ok {
		msg := &QmpError{}
		err = json.Unmarshal(raw, msg)
		qmp.msg = msg
	}
	return err
}

func qmpFail(err string, callback QemuEvent) *QmpFinish {
	return &QmpFinish{
		success:  false,
		reason:   map[string]interface{}{"error": err},
		callback: callback,
	}
}

func qmpReceiver(ch chan QmpInteraction, decoder *json.Decoder) {
	glog.V(0).Info("Begin receive QMP message")
	for {
		rsp := &QmpResponse{}
		if err := decoder.Decode(rsp); err == io.EOF {
			glog.Info("QMP exit as got EOF")
			ch <- &QmpInternalError{cause: err.Error()}
			return
		} else if err != nil {
			glog.Error("QMP receive and decode error: ", err.Error())
			ch <- &QmpInternalError{cause: err.Error()}
			return
		}
		msg := rsp.msg
		ch <- msg

		if msg.MessageType() == QMP_EVENT && msg.(*QmpEvent).Type == QMP_EVENT_SHUTDOWN {
			glog.V(0).Info("Shutdown, quit QMP receiver")
			return
		}
	}
}

func qmpInitializer(ctx *VmContext) {
	conn, err := unixSocketConnect(ctx.qmpSockName)
	if err != nil {
		glog.Error("failed to connected to ", ctx.qmpSockName, " ", err.Error())
		ctx.qmp <- qmpFail(err.Error(), nil)
		return
	}

	glog.V(1).Info("connected to ", ctx.qmpSockName)

	var msg map[string]interface{}
	decoder := json.NewDecoder(conn)
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	glog.Info("begin qmp init...")

	err = decoder.Decode(&msg)
	if err != nil {
		glog.Error("get qmp welcome failed: ", err.Error())
		ctx.qmp <- qmpFail(err.Error(), nil)
		return
	}

	glog.Info("got qmp welcome, now sending command qmp_capabilities")

	cmd, err := json.Marshal(QmpCommand{Execute: "qmp_capabilities"})
	if err != nil {
		glog.Error("qmp_capabilities marshal failed ", err.Error())
		ctx.qmp <- qmpFail(err.Error(), nil)
		return
	}
	_, err = conn.Write(cmd)
	if err != nil {
		glog.Error("qmp_capabilities send failed ", err.Error())
		ctx.qmp <- qmpFail(err.Error(), nil)
		return
	}

	glog.Info("waiting for response")
	rsp := &QmpResponse{}
	err = decoder.Decode(rsp)
	if err != nil {
		glog.Error("response receive failed ", err.Error())
		ctx.qmp <- qmpFail(err.Error(), nil)
		return
	}

	glog.Info("got for response")

	if rsp.msg.MessageType() == QMP_RESULT {
		glog.Info("QMP connection initialized")
		ctx.qmp <- &QmpInit{
			conn:    conn.(*net.UnixConn),
			decoder: decoder,
		}
		return
	}

	ctx.qmp <- qmpFail("handshake failed", nil)
}

func qmpCommander(handler chan QmpInteraction, conn *net.UnixConn, session *QmpSession, feedback chan QmpInteraction) {
	glog.V(1).Info("Begin process command session")
	for _, cmd := range session.commands {
		msg, err := json.Marshal(*cmd)
		if err != nil {
			handler <- qmpFail("cannot marshal command", session.callback)
			return
		}

		success := false
		var qe *QmpError = nil
		for repeat := 0; !success && repeat < 3; repeat++ {

			if len(cmd.Scm) > 0 {
				glog.V(1).Infof("send cmd with scm (%d bytes) (%d) %s", len(cmd.Scm), repeat+1, string(msg))
				f, _ := conn.File()
				fd := f.Fd()
				syscall.Sendmsg(int(fd), msg, cmd.Scm, nil, 0)
			} else {
				glog.V(1).Infof("sending command (%d) %s", repeat+1, string(msg))
				conn.Write(msg)
			}

			res, ok := <-feedback
			if !ok {
				glog.Info("QMP command result chan closed")
				return
			}
			switch res.MessageType() {
			case QMP_RESULT:
				success = true
				break
			//success
			case QMP_ERROR:
				glog.Warning("got one qmp error")
				qe = res.(*QmpError)
				time.Sleep(1000 * time.Millisecond)
			case QMP_INTERNAL_ERROR:
				glog.Info("QMP quit... commander quit... ")
				return
			}
		}

		if !success {
			handler <- qe.Finish(session.callback)
			return
		}
	}
	handler <- session.Finish()
	return
}

func qmpHandler(ctx *VmContext) {

	go qmpInitializer(ctx)

	timer := time.AfterFunc(10*time.Second, func() {
		glog.Warning("Initializer Timeout.")
		ctx.qmp <- &QmpTimeout{}
	})

	type msgHandler func(QmpInteraction)
	var handler msgHandler = nil
	var conn *net.UnixConn = nil

	buf := []*QmpSession{}
	res := make(chan QmpInteraction, 128)

	loop := func(msg QmpInteraction) {
		switch msg.MessageType() {
		case QMP_SESSION:
			glog.Info("got new session")
			buf = append(buf, msg.(*QmpSession))
			if len(buf) == 1 {
				go qmpCommander(ctx.qmp, conn, msg.(*QmpSession), res)
			}
		case QMP_FINISH:
			glog.Infof("session finished, buffer size %d", len(buf))
			r := msg.(*QmpFinish)
			if r.success {
				glog.V(1).Info("success ")
				if r.callback != nil {
					ctx.hub <- r.callback
				}
			} else {
				reason := "unknown"
				if c, ok := r.reason["error"]; ok {
					reason = c.(string)
				}
				glog.Error("QMP command failed ", reason)
				ctx.hub <- &DeviceFailed{
					session: r.callback,
				}
			}
			buf = buf[1:]
			if len(buf) > 0 {
				go qmpCommander(ctx.qmp, conn, buf[0], res)
			}
		case QMP_RESULT, QMP_ERROR:
			res <- msg
		case QMP_EVENT:
			ev := msg.(*QmpEvent)
			ctx.hub <- ev
			if ev.Type == QMP_EVENT_SHUTDOWN {
				glog.Info("got QMP shutdown event, quit...")
				handler = nil
			}
		case QMP_INTERNAL_ERROR:
			res <- msg
			handler = nil
			glog.Info("QMP handler quit as received ", msg.(*QmpInternalError).cause)
			ctx.hub <- &Interrupted{reason: msg.(*QmpInternalError).cause}
		case QMP_QUIT:
			handler = nil
		}
	}

	initializing := func(msg QmpInteraction) {
		switch msg.MessageType() {
		case QMP_INIT:
			timer.Stop()
			init := msg.(*QmpInit)
			conn = init.conn
			handler = loop
			glog.Info("QMP initialzed, go into main QMP loop")

			//routine for get message
			go qmpReceiver(ctx.qmp, init.decoder)
			if len(buf) > 0 {
				go qmpCommander(ctx.qmp, conn, buf[0], res)
			}
		case QMP_FINISH:
			finish := msg.(*QmpFinish)
			if !finish.success {
				timer.Stop()
				ctx.hub <- &InitFailedEvent{
					reason: finish.reason["error"].(string),
				}
				handler = nil
				glog.Error("QMP initialize failed")
			}
		case QMP_TIMEOUT:
			ctx.hub <- &InitFailedEvent{
				reason: "QMP Init timeout",
			}
			handler = nil
			glog.Error("QMP initialize timeout")
		case QMP_SESSION:
			glog.Info("got new session during initializing")
			buf = append(buf, msg.(*QmpSession))
		}
	}

	handler = initializing

	for handler != nil {
		msg, ok := <-ctx.qmp
		if !ok {
			glog.Info("QMP channel closed, Quit qmp handler")
			break
		}
		handler(msg)
	}
}
