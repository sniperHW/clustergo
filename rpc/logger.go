package rpc

import (
	"github.com/sniperHW/clustergo/logger"
)

// Logger 是 logger.Logger 的别名，具体定义在 logger 包。
// 保留该别名是为了兼容旧代码（字段、函数签名里写 rpc.Logger 的地方）。
type Logger = logger.Logger

// InitLogger 设置全局日志器，与 clustergo.InitLogger 操作的是同一个实例。
//
// Deprecated: 统一在 logger 包注入（clustergo.InitLogger 或 logger.InitLogger），
// 该函数仅为兼容旧调用点保留。
func InitLogger(l Logger) {
	logger.InitLogger(l)
}
