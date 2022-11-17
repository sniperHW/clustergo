package discovery

import (
	"github.com/sniperHW/clustergo/addr"
)

type Node struct {
	Addr      addr.Addr
	Export    bool //是否将节点暴露到cluster外部
	Available bool //是否可用,
}

type Discovery interface {
	//获取节点信息
	//LoadNodeInfo() ([]Node, error)
	//订阅变更
	Subscribe(func([]Node)) error
}
