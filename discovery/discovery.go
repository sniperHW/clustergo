package discovery

import (
	"github.com/sniperHW/sanguo/addr"
)

type Discovery interface {
	//获取节点信息
	LoadNodeInfo() ([]addr.Addr, error)
	//订阅变更
	Subscribe(func([]addr.Addr))
}
