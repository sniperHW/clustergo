package rpc

// Logger is the logging interface used by the rpc package. It matches
// clustergo.Logger exactly so the same logger instance can be shared.
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

// InitLogger sets the package logger. Call from clustergo.InitLogger.
func InitLogger(l Logger) {
	logger = l
}
