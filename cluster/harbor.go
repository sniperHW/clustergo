package cluster

import (
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
)

const harbarType uint32 = 255

var harborsByGroup map[uint32][]*endPoint = map[uint32][]*endPoint{}

func isSelfHarbor() bool {
	return selfAddr.Logic.Type() == harbarType
}

func getHarbor() *endPoint {
	return getHarborByGroup(selfAddr.Logic.Group())
}

func getHarborByGroup(group uint32) *endPoint {
	mtx.Lock()
	defer mtx.Unlock()
	harborGroup, ok := harborsByGroup[group]
	if ok && len(harborGroup) > 0 {
		return harborGroup[0]
	} else {
		return nil
	}
}

func addHarbor(harbor *endPoint) {

	group := harbor.addr.Logic.Group()

	harborGroup, ok := harborsByGroup[group]
	if !ok {
		harborsByGroup[group] = []*endPoint{harbor}
	} else {
		for _, v := range harborsByGroup[group] {
			if v.addr.Logic == harbor.addr.Logic {
				return
			}
		}
		harborsByGroup[group] = append(harborGroup, harbor)
	}

	Infoln("addHarbor", harbor.addr.Logic.String(), harbor)

}

func removeHarbor(addr addr.LogicAddr) {
	group := addr.Group()
	harborGroup, ok := harborsByGroup[group]
	if ok && len(harborGroup) > 0 {
		i := 0
		for ; i < len(harborGroup); i++ {
			if harborGroup[i].addr.Logic == addr {
				break
			}
		}
		if i != len(harborGroup) {
			harbor := harborGroup[i]
			if nil != harbor.conn {
				harbor.conn.session.Close("remove harbor", 0)
			}
			harborGroup[i], harborGroup[len(harborGroup)-1] = harborGroup[len(harborGroup)-1], harborGroup[i]
			harborsByGroup[group] = harborGroup[:len(harborGroup)-1]
		}
	}
}

func postRelayError(peer addr.LogicAddr, msg *ss.RCPRelayErrorMessage) {

	endPoint := getEndPoint(peer)
	if nil == endPoint {
		endPoint = getHarborByGroup(peer.Group())
		if endPoint.addr.Logic == selfAddr.Logic {
			Errorln("postRelayError ring!!!")
			return
		}
	}

	if nil != endPoint {
		endPoint.mtx.Lock()
		defer endPoint.mtx.Unlock()
		if nil != endPoint.conn {
			endPoint.conn.session.Send(msg)
		} else {
			endPoint.pendingMsg = append(endPoint.pendingMsg, msg)
			//尝试与对端建立连接
			dial(endPoint)
		}
	} else {
		Errorf("postRelayError %s not found", peer.String())
	}
}

func onRelayError(message *ss.RelayMessage, err string) {
	if message.IsRPCReq() {
		//通告请求端消息无法送达到目的地
		msg := &ss.RCPRelayErrorMessage{
			To:     message.From,
			From:   selfAddr.Logic,
			Seqno:  message.GetSeqno(),
			ErrMsg: err,
		}

		postRelayError(message.From, msg)
	}
}

func onRelayMessage(message *ss.RelayMessage) {
	endPoint := getEndPoint(message.To)
	if nil == endPoint {
		endPoint = getHarborByGroup(message.To.Group())
		if endPoint.addr.Logic == selfAddr.Logic {
			Errorln("onRelayMessage ring!!!")
			return
		}
	}

	if nil != endPoint {
		endPoint.mtx.Lock()
		defer endPoint.mtx.Unlock()

		if nil != endPoint.conn {
			endPoint.conn.session.Send(message)
		} else {
			endPoint.pendingMsg = append(endPoint.pendingMsg, message)
			//尝试与对端建立连接
			dial(endPoint)
		}
	} else {
		onRelayError(message, "unable route to target")
	}
}

func onEndPointJoin(end *endPoint) {
	if isSelfHarbor() {
		if end.addr.Logic.Type() == harbarType {
			if end.addr.Logic.Group() != selfAddr.Logic.Group() {
				//新节点不是本组内的harbor,将本组内非harbor节点通告给它
				forgins := []uint32{}

				for tt, v := range ttForignServiceMap {
					if tt != harbarType {
						for _, vv := range v.services {
							forgins = append(forgins, uint32(vv))
						}
					}
				}

				postToEndPoint(end, &AddForginServicesH2H{Nodes: forgins})
			}
		} else {
			//Infoln("onEndPointJoin", end.addr.Logic.String(), selfAddr.Logic.String())
			if end.exportService == 1 {
				//通告非本group的其它harbor,有新节点加入,需要添加forginService
				for g, v := range harborsByGroup {
					if g != selfAddr.Logic.Group() {
						for _, vv := range v {
							postToEndPoint(vv, &AddForginServicesH2H{Nodes: []uint32{uint32(end.addr.Logic)}})
						}
					}
				}
			}

			//向新节点发送已知的forginService
			forgins := []uint32{}

			for tt, v := range ttForignServiceMap {
				if tt != harbarType {
					for _, vv := range v.services {
						forgins = append(forgins, uint32(vv))
					}
				}
			}

			postToEndPoint(end, &AddForginServicesH2S{Nodes: forgins})

		}
	}
}

func onEndPointLeave(end *endPoint) {
	if isSelfHarbor() && end.addr.Logic.Type() != harbarType {
		Infoln("onEndPointLeave")
		//通告非本group的其它harbor,有节点离开,需要移除forginService
		for g, v := range harborsByGroup {
			if g != selfAddr.Logic.Group() {
				for _, vv := range v {
					postToEndPoint(vv, &RemForginServicesH2H{Nodes: []uint32{uint32(end.addr.Logic)}})
				}
			}
		}
	}
}

func onAddForginServicesH2H(msg *AddForginServicesH2H) {
	if isSelfHarbor() {
		for _, v := range msg.GetNodes() {
			addForginService(addr.LogicAddr(v))
		}

		//向本group内节点同步forginServices
		BrocastToAll(&AddForginServicesH2S{Nodes: msg.GetNodes()}, harbarType)

	}
}

func onRemForginServicesH2H(msg *RemForginServicesH2H) {
	if isSelfHarbor() {
		for _, v := range msg.GetNodes() {
			removeForginService(addr.LogicAddr(v))
		}
		//向本group内节点同步rem forginServices
		BrocastToAll(&RemForginServicesH2S{Nodes: msg.GetNodes()}, harbarType)
	}
}
