package cluster

import (
	//"net"
	//center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
)

const harbarType uint32 = 255

var harborsByGroup map[uint32][]*endPoint = map[uint32][]*endPoint{}

func getHarbor() *endPoint {
	return getHarborByGroup(selfAddr.Logic.Group())
}

func getHarborByGroup(group uint32) *endPoint {
	mtx.Lock()
	defer mtx.Unlock()
	//Debugln("getHarbor", group)
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

	endPoint.mtx.Lock()
	defer endPoint.mtx.Unlock()

	if nil != endPoint {
		if nil != endPoint.conn {
			err := endPoint.conn.session.Send(msg)
			if nil != err {
				Errorln("postRelayError error:", err.Error(), peer.String())
			}
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

	endPoint.mtx.Lock()
	defer endPoint.mtx.Unlock()

	if nil != endPoint {
		if nil != endPoint.conn {
			err := endPoint.conn.session.Send(message)
			if nil != err {
				onRelayError(message, err.Error())
			}
		} else {
			endPoint.pendingMsg = append(endPoint.pendingMsg, message)
			//尝试与对端建立连接
			dial(endPoint)
		}
	} else {
		onRelayError(message, "unable route to target")
	}
}
