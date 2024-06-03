package membership

import (
	"github.com/sniperHW/clustergo/addr"
)

type Node struct {
	Addr      addr.Addr
	Export    bool //是否将节点暴露到cluster外部
	Available bool //是否可用,
}

type MemberInfo struct {
	Add    []Node
	Remove []Node
	Update []Node
}

type Client interface {
	//订阅变更
	Subscribe(func(MemberInfo)) error
	Close()
}

type Admin interface {
	//更新节点信息，如果节点不存在将它添加到membership中
	UpdateMember(Node)
	//从membership中移除节点
	RemoveMember(Node)
}
