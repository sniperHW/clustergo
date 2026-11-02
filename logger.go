package clustergo

import (
	"github.com/sniperHW/clustergo/logger"
)

// Logger 是日志接口的别名，具体定义在 logger 包，clustergo 与 rpc 共用同一套接口。
type Logger = logger.Logger

// InitLogger 设置全局日志器，clustergo 与 rpc 等子包共用这一个实例。
// *zap.SugaredLogger（logger/zap 下 NewZapLogger(...).Sugar()）满足 Logger 接口。
func InitLogger(l Logger) {
	logger.InitLogger(l)
}

// Log 返回当前全局日志器；未调用 InitLogger 时返回一个丢弃日志的实现。
func Log() Logger {
	return logger.Get()
}
