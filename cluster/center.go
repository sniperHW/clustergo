package cluster

import (
	"github.com/golang/protobuf/proto"
	"github.com/kr/pretty"
	"github.com/sniperHW/kendynet"
	center "github.com/sniperHW/sanguo/center"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"sort"
)

const EXPORT bool = true

func connectCenter(centerAddrs []string, selfAddr addr.Addr, export ...bool) {

	exportService := uint32(0)

	if len(export) > 0 && export[0] {
		exportService = 1
	}

	center.ClientInit(GetEventQueue(), logger, exportService)

	center.ConnectCenter(centerAddrs, selfAddr)
}

func diff(a, b []*center_proto.NodeInfo) ([]*center_proto.NodeInfo, []*center_proto.NodeInfo) {

	if len(a) == 0 {
		return nil, b
	}

	if len(b) == 0 {
		return a, nil
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].GetLogicAddr() < a[j].GetLogicAddr()
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].GetLogicAddr() < b[j].GetLogicAddr()
	})

	add := []*center_proto.NodeInfo{}
	remove := []*center_proto.NodeInfo{}

	i := 0
	j := 0

	for i < len(a) && j < len(b) {
		if a[i].GetLogicAddr() == b[j].GetLogicAddr() {
			add = append(add, a[i])
			i++
			j++
		} else if a[i].GetLogicAddr() > b[j].GetLogicAddr() {
			remove = append(remove, b[j])
			j++
		} else {
			add = append(add, a[i])
			i++
		}
	}

	if len(a[i:]) > 0 {
		add = append(add, a[i:]...)
	}

	if len(b[j:]) > 0 {
		remove = append(remove, b[j:]...)
	}

	return add, remove
}

func centerInit() {
	center.RegisterCenterMsgHandler(center_proto.CENTER_HeartbeatToNode, func(session kendynet.StreamSession, msg proto.Message) {
		//心跳响应暂时不处理
		//kendynet.Infof("HeartbeatToNode\n")
	})

	center.RegisterCenterMsgHandler(center_proto.CENTER_NotifyNodeAdd, func(session kendynet.StreamSession, msg proto.Message) {
		Infoln("CENTER_NotifyNodeAdd", pretty.Sprint(msg))

		NodeAdd := msg.(*center_proto.NodeAdd)

		for _, v := range NodeAdd.Nodes {
			addEndPoint(session.RemoteAddr(), v)
		}

	})

	center.RegisterCenterMsgHandler(center_proto.CENTER_NotifyNodeInfo, func(session kendynet.StreamSession, msg proto.Message) {
		Infoln("CENTER_NotifyNodeInfo", pretty.Sprint(msg))
		mtx.Lock()

		currentEndPoints := []*center_proto.NodeInfo{}

		for k, _ := range idEndPointMap {
			currentEndPoints = append(currentEndPoints, &center_proto.NodeInfo{
				LogicAddr: proto.Uint32(uint32(k)),
			})
		}

		mtx.Unlock()

		NotifyNodeInfo := msg.(*center_proto.NotifyNodeInfo)

		add, remove := diff(NotifyNodeInfo.Nodes, currentEndPoints)

		for _, v := range add {
			addEndPoint(session.RemoteAddr(), v)
		}

		for _, v := range remove {
			removeEndPoint(session.RemoteAddr(), addr.LogicAddr(v.GetLogicAddr()))
		}

	})

	center.RegisterCenterMsgHandler(center_proto.CENTER_NodeLeave, func(session kendynet.StreamSession, msg proto.Message) {
		NodeLeave := msg.(*center_proto.NodeLeave)
		for _, v := range NodeLeave.Nodes {
			removeEndPoint(session.RemoteAddr(), addr.LogicAddr(v))
		}
	})
}
