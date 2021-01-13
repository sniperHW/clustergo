package common

import (
	"time"
)

var (
	HeartBeat_Timeout        = time.Duration(120 * time.Second) //心跳超时时间
	HeartBeat_Timeout_Client = time.Duration(120 * time.Second) //客户端
	ReconnectTime            = time.Duration(600 * time.Second) //断线超时时间
)
