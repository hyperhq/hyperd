package hypervisor

import (
	"github.com/hyperhq/hypercontainer-utils/hlog"
)

const (
	ERROR   = hlog.ERROR
	WARNING = hlog.WARNING
	INFO    = hlog.INFO
	DEBUG   = hlog.DEBUG
	TRACE   = hlog.TRACE
	EXTRA   = hlog.EXTRA
)

func (ctx *VmContext) LogLevel(level hlog.LogLevel) bool {
	return hlog.IsLogLevel(level)
}

func (ctx *VmContext) LogPrefix() string {
	if ctx == nil {
		return "SB[] "
	}
	return ctx.logPrefix
}

func (ctx *VmContext) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, ctx, 1, args...)
}

func (cc *ContainerContext) LogPrefix() string {
	if cc == nil {
		return "SB[] Con[] "
	}
	return cc.logPrefix
}

func (cc *ContainerContext) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, cc, 1, args...)
}
