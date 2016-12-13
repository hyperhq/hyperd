package json

import (
	"encoding/binary"
	"syscall"
)

type FileCommand struct {
	Container string `json:"container"`
	File      string `json:"file"`
}

type KillCommand struct {
	Container string         `json:"container"`
	Signal    syscall.Signal `json:"signal"`
}

type ExecCommand struct {
	Container string  `json:"container,omitempty"`
	Process   Process `json:"process"`
}

type Routes struct {
	Routes []Route `json:"routes,omitempty"`
}

// Message
type DecodedMessage struct {
	Code    uint32
	Message []byte
}

type TtyMessage struct {
	Session uint64
	Message []byte
}

func (tm *TtyMessage) ToBuffer() []byte {
	length := len(tm.Message) + 12
	buf := make([]byte, length)
	binary.BigEndian.PutUint64(buf[:8], tm.Session)
	binary.BigEndian.PutUint32(buf[8:12], uint32(length))
	copy(buf[12:], tm.Message)
	return buf
}

type WindowSizeMessage struct {
	Container string `json:"container"`
	Process   string `json:"process"`
	Row       uint16 `json:"row"`
	Column    uint16 `json:"column"`
}

type ProcessAsyncEvent struct {
	Container string `json:"container"`
	Process   string `json:"process"`
	Event     string `json:"event"` // curent supported events: finished
	Info      string `json:"info,omitempty"`
	Status    int    `json:"status,omitempty"`
}
