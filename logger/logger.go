// Package logger 定义 clustergo 及其子包（rpc 等）共用的日志接口与全局日志器。
//
// 使用方通过 InitLogger 注入任意实现（*zap.SugaredLogger 直接满足 Logger），
// 包内与各子包统一调用本包的包级函数（如 logger.Errorf）写日志，
// 保证整个进程只有一份日志配置。
package logger

import (
	"fmt"
	"sync/atomic"
)

// Logger 是 clustergo 与 rpc 共用的日志接口。
//
// *zap.SugaredLogger 满足该接口；业务方也可以自行实现后注入。
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

// nopLogger 是未调用 InitLogger 时使用的兜底实现。
// Debug/Info/Warn/Error 直接丢弃，避免库在未配置日志时 panic；
// Panic/Fatal 仍然 panic，保留"编程错误必须暴露"的语义。
type nopLogger struct{}

func (nopLogger) Debugf(string, ...interface{}) {}
func (nopLogger) Infof(string, ...interface{})  {}
func (nopLogger) Warnf(string, ...interface{})  {}
func (nopLogger) Errorf(string, ...interface{}) {}
func (nopLogger) Debug(...interface{})          {}
func (nopLogger) Info(...interface{})           {}
func (nopLogger) Warn(...interface{})           {}
func (nopLogger) Error(...interface{})          {}

func (nopLogger) Panicf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

func (nopLogger) Fatalf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

func (nopLogger) Panic(args ...interface{}) {
	panic(fmt.Sprint(args...))
}

func (nopLogger) Fatal(args ...interface{}) {
	panic(fmt.Sprint(args...))
}

// current 保存全局日志器；nil 表示尚未初始化，读取时回退到 nopLogger。
var current atomic.Pointer[Logger]

// InitLogger 注入全局日志器，clustergo 与 rpc 等子包共用这一个实例。
// 传入 nil 时回退为丢弃日志的实现（Panic/Fatal 仍会 panic）。
// 可在运行时安全地再次调用替换日志器。
func InitLogger(l Logger) {
	if l == nil {
		current.Store(nil)
		return
	}
	current.Store(&l)
}

// Get 返回当前全局日志器，未初始化时返回丢弃日志的实现。
func Get() Logger {
	if p := current.Load(); p != nil {
		return *p
	}
	return nopLogger{}
}

func Debugf(format string, args ...interface{}) { Get().Debugf(format, args...) }
func Infof(format string, args ...interface{})  { Get().Infof(format, args...) }
func Warnf(format string, args ...interface{})  { Get().Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { Get().Errorf(format, args...) }
func Panicf(format string, args ...interface{}) { Get().Panicf(format, args...) }
func Fatalf(format string, args ...interface{}) { Get().Fatalf(format, args...) }

func Debug(args ...interface{}) { Get().Debug(args...) }
func Info(args ...interface{})  { Get().Info(args...) }
func Warn(args ...interface{})  { Get().Warn(args...) }
func Error(args ...interface{}) { Get().Error(args...) }
func Panic(args ...interface{}) { Get().Panic(args...) }
func Fatal(args ...interface{}) { Get().Fatal(args...) }
