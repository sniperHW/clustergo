package cluster

import (
	"github.com/golang/protobuf/proto"
	//	"github.com/kr/pretty"
	"github.com/sniperHW/kendynet"
	center_client "github.com/sniperHW/sanguo/center/client"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"sort"
	"strings"
)

func (this *Cluster) connectCenter(centerAddrs []string) {
	this.centerClient.ConnectCenter(centerAddrs, this.serverState.selfAddr)
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

func (this *Cluster) centerInit(export ...bool) {

	exportService := uint32(0)

	if len(export) > 0 && export[0] {
		exportService = 1
	}

	this.centerClient = center_client.New(this.queue, logger, exportService)

	this.centerClient.RegisterCenterMsgHandler(center_proto.CENTER_HeartbeatToNode, func(session kendynet.StreamSession, msg proto.Message) {
		//心跳响应暂时不处理
		//kendynet.Infof("HeartbeatToNode\n")
	})

	nodes2Str := func(nodes []*center_proto.NodeInfo) string {
		s := []string{}
		for _, v := range nodes {
			t := addr.LogicAddr(v.GetLogicAddr())
			s = append(s, t.String())
		}

		return strings.Join(s, ",")
	}

	this.centerClient.RegisterCenterMsgHandler(center_proto.CENTER_NotifyNodeAdd, func(session kendynet.StreamSession, msg proto.Message) {
		NodeAdd := msg.(*center_proto.NodeAdd)

		logger.Infoln(this.serverState.selfAddr.Logic.String(), "CENTER_NotifyNodeAdd", nodes2Str(NodeAdd.Nodes))

		for _, v := range NodeAdd.Nodes {
			this.serviceMgr.addEndPoint(session.RemoteAddr(), v)
		}

	})

	this.centerClient.RegisterCenterMsgHandler(center_proto.CENTER_NotifyNodeInfo, func(session kendynet.StreamSession, msg proto.Message) {

		currentEndPoints := this.serviceMgr.getAllNodeInfo()

		NotifyNodeInfo := msg.(*center_proto.NotifyNodeInfo)

		logger.Infoln(this.serverState.selfAddr.Logic.String(), "CENTER_NotifyNodeInfo", nodes2Str(NotifyNodeInfo.Nodes))

		add, remove := diff(NotifyNodeInfo.Nodes, currentEndPoints)

		for _, v := range add {
			this.serviceMgr.addEndPoint(session.RemoteAddr(), v)
		}

		/*
		 *  在单一center的配置下，如果center crash之后重启，节点重新连接center,此时原集群中可能尚有部分节点未连接上center
		 *  此时，NotifyNodeInfo将只包含部分节点。例如集群中原有1,2,3,4,4个节点,center崩溃重启后,1先连上center,此时收到的NotifyNodeInfo只有1。
		 *  diff将把2,3,4计算到remove中。
		 *
		 *  如果直接将2,3,4删除，将导致到1与这些节点的通信暂时中断，直到2,3,4连上center并把信息发送到1。
		 *
		 *  为了避免这个中断，endpoint中添加了timestamp,当调用addEndPoint将更新timestamp。
		 *  然后调用delayRemove,把本次要remove的end和时间记录下来，延迟到一定时间之后再执行。如果在延迟执行之前，2，3，4连上center并通告到1，1中对应的
		 *  end将会更新timestamp，延迟执行时会比较end.timestamp和记录下的timestamp,如果end.timestamp更新将放弃删除。
		 *
		 */

		this.serviceMgr.delayRemove(session.RemoteAddr(), remove)

	})

	this.centerClient.RegisterCenterMsgHandler(center_proto.CENTER_NodeLeave, func(session kendynet.StreamSession, msg proto.Message) {
		NodeLeave := msg.(*center_proto.NodeLeave)
		//logger.Infoln("CENTER_NodeLeave", pretty.Sprint(msg))
		for _, v := range NodeLeave.Nodes {
			this.serviceMgr.removeEndPoint(session.RemoteAddr(), addr.LogicAddr(v))
		}
	})
}
