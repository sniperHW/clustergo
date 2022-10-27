package sanguo

import (
	"github.com/sniperHW/rpcgo"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
	Panicf(string, ...interface{})
	Fatalf(string, ...interface{})
	Debug(...interface{})
	Info(...interface{})
	Warn(...interface{})
	Error(...interface{})
	Panic(...interface{})
	Fatal(...interface{})
}

var logger Logger

func InitLogger(l Logger) {
	rpcgo.InitLogger(l.(rpcgo.Logger))
	logger = l
}

func Log() Logger {
	return logger
}
