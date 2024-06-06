package membership

import (
	"encoding/json"

	"github.com/sniperHW/clustergo/addr"
)

type nodeJson struct {
	LogicAddr string `json:"logicAddr"`
	NetAddr   string `json:"netAddr"`
	Export    bool   `json:"export"`
	Available bool   `json:"available"`
}

type Node struct {
	Addr      addr.Addr
	Export    bool //是否将节点暴露到cluster外部
	Available bool //是否可用,
}

func (n *Node) Marshal() ([]byte, error) {
	j := nodeJson{
		Export:    n.Export,
		Available: n.Available,
		LogicAddr: n.Addr.LogicAddr().String(),
		NetAddr:   n.Addr.NetAddr().String(),
	}
	return json.Marshal(j)
}

func (n *Node) Unmarshal(data []byte) (err error) {
	var j nodeJson
	if err = json.Unmarshal(data, &j); err != nil {
		return
	}

	n.Available = j.Available
	n.Export = j.Export
	n.Addr, err = addr.MakeAddr(j.LogicAddr, j.NetAddr)
	return
}

type MemberInfo struct {
	Add    []Node
	Remove []Node
	Update []Node
}

type Subscribe interface {
	//订阅变更
	Subscribe(func(MemberInfo)) error
	Close()
}

type Admin interface {
	//更新节点信息，如果节点不存在将它添加到membership中
	UpdateMember(Node)
	//从membership中移除节点
	RemoveMember(Node)
	//保活
	KeepAlive(Node)
}
