package cluster

import (
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"sort"
	"time"
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

	Infoln("addHarbor", harbor.addr.Logic.String())

}

func removeHarbor(harbor *endPoint) {
	if nil != harbor.session {
		harbor.session.Close("remove harbor", 0)
	}
	group := harbor.addr.Logic.Group()
	harborGroup, ok := harborsByGroup[group]
	if ok && len(harborGroup) > 0 {
		i := 0
		for ; i < len(harborGroup); i++ {
			if harborGroup[i].addr.Logic == harbor.addr.Logic {
				break
			}
		}
		if i != len(harborGroup) {
			harborGroup[i], harborGroup[len(harborGroup)-1] = harborGroup[len(harborGroup)-1], harborGroup[i]
			harborsByGroup[group] = harborGroup[:len(harborGroup)-1]
		}
	}
}

func postRelayError(peer addr.LogicAddr, msg *ss.RCPRelayErrorMessage) {

	endPoint := getEndPoint(peer)
	if nil == endPoint {
		endPoint = getHarborByGroup(peer.Group())
		if nil != endPoint && endPoint.addr.Logic == selfAddr.Logic {
			Errorln("postRelayError ring!!!")
			return
		}
	}

	if nil != endPoint {
		endPoint.mtx.Lock()
		defer endPoint.mtx.Unlock()
		if nil != endPoint.session {
			endPoint.session.Send(msg)
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

		Errorln("onRelayError", err)

		postRelayError(message.From, msg)
	}
}

func onRelayMessage(message *ss.RelayMessage) {
	endPoint := getEndPoint(message.To)
	if nil == endPoint {
		endPoint = getHarborByGroup(message.To.Group())
		if nil != endPoint && endPoint.addr.Logic == selfAddr.Logic {
			Errorln("onRelayMessage ring!!!")
			return
		}
	}

	if nil != endPoint {
		endPoint.mtx.Lock()
		defer endPoint.mtx.Unlock()

		if nil != endPoint.session {
			endPoint.session.Send(message)
		} else {
			endPoint.pendingMsg = append(endPoint.pendingMsg, message)
			//尝试与对端建立连接
			dial(endPoint)
		}
	} else {
		Infoln("unable route to target", message.To)
		onRelayError(message, "unable route to target")
	}
}

func onEndPointJoin(end *endPoint) {
	if isSelfHarbor() {
		if end.addr.Logic.Type() == harbarType {
			if end.addr.Logic.Group() != selfAddr.Logic.Group() {
				//新节点不是本组内的harbor,将本组内非harbor节点通告给它
				forgins := []uint32{}

				for k, v := range idEndPointMap {
					if v.exportService == 1 {
						forgins = append(forgins, uint32(k))
					}
				}

				NotifyForginServicesH2H(end, forgins)
			}
		} else {
			if end.exportService == 1 {
				Infoln("exportService", end.addr.Logic.String(), "join")
				//通告非本group的其它harbor,有新节点加入,需要添加forginService
				for g, v := range harborsByGroup {
					if g != selfAddr.Logic.Group() {
						for _, vv := range v {
							AddForginServicesH2H(vv, []uint32{uint32(end.addr.Logic)})
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

			NotifyForginServicesH2S(end, forgins)
		}
	}
}

func onEndPointLeave(end *endPoint) {
	if isSelfHarbor() && end.addr.Logic.Type() != harbarType {
		//通告非本group的其它harbor,有节点离开,需要移除forginService
		for g, v := range harborsByGroup {
			if g != selfAddr.Logic.Group() {
				for _, vv := range v {
					RemForginServicesH2H(vv, []uint32{uint32(end.addr.Logic)})
				}
			}
		}
	}
}

func NotifyForginServicesH2H(end *endPoint, nodes []uint32) {
	req := &NotifyForginServicesH2HReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("NotifyForginServicesH2H to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("NotifyForginServicesH2H to ", end.addr.Logic, "error", err, "try again")
					NotifyForginServicesH2H(end, nodes)
				}
			}()
		}
	})
}

func AddForginServicesH2H(end *endPoint, nodes []uint32) {
	req := &AddForginServicesH2HReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("AddForginServicesH2H to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("AddForginServicesH2H to ", end.addr.Logic, "error", err, "try again")
					AddForginServicesH2H(end, nodes)
				}
			}()
		}
	})
}

func NotifyForginServicesH2S(end *endPoint, nodes []uint32) {
	req := &NotifyForginServicesH2SReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("NotifyForginServicesH2S to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("NotifyForginServicesH2S to ", end.addr.Logic, "error", err, "try again")
					NotifyForginServicesH2S(end, nodes)
				}
			}()
		}
	})
}

func AddForginServicesH2S(end *endPoint, nodes []uint32) {
	req := &AddForginServicesH2SReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("AddForginServicesH2S to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("AddForginServicesH2S to ", end.addr.Logic, "error", err, "try again")
					AddForginServicesH2S(end, nodes)
				}
			}()
		}
	})
}

func RemForginServicesH2H(end *endPoint, nodes []uint32) {
	req := &RemForginServicesH2HReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("RemForginServicesH2H to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("RemForginServicesH2H to ", end.addr.Logic, "error", err, "try again")
					RemForginServicesH2H(end, nodes)
				}
			}()
		}
	})
}

func RemForginServicesH2S(end *endPoint, nodes []uint32) {
	req := &RemForginServicesH2SReq{Nodes: nodes}
	asynCall(end, req, 1000, func(_ interface{}, err error) {
		if nil == err {
			Infoln("RemForginServicesH2S to ", end.addr.Logic, "ok")
		} else if nil != err {
			go func() {
				time.Sleep(time.Second)
				if end == getEndPoint(end.addr.Logic) {
					Infoln("RemForginServicesH2S to ", end.addr.Logic, "error", err, "try again")
					RemForginServicesH2S(end, nodes)
				}
			}()
		}
	})
}

func diff2(a, b []uint32) ([]uint32, []uint32) {

	if len(a) == 0 {
		return nil, b
	}

	if len(b) == 0 {
		return a, nil
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i] < a[j]
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i] < b[j]
	})

	add := []uint32{}
	remove := []uint32{}

	i := 0
	j := 0

	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			add = append(add, a[i])
			i++
			j++
		} else if a[i] > b[j] {
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

func init() {

	RegisterMethod(&NotifyForginServicesH2HReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("NotifyForginServicesH2HReq")
		if isSelfHarbor() {

			msg := req.(*NotifyForginServicesH2HReq)

			current := []uint32{}

			endPoints := []*endPoint{}

			Infoln("NotifyForginServicesH2HReq", msg.GetNodes())

			mtx.Lock()
			for _, v1 := range ttForignServiceMap {
				for _, v2 := range v1.services {
					current = append(current, uint32(v2))
				}
			}

			for tt, v := range ttEndPointMap {
				if tt != harbarType {
					for _, vv := range v.endPoints {
						endPoints = append(endPoints, vv)
					}
				}
			}

			mtx.Unlock()

			add, remove := diff2(msg.GetNodes(), current)

			for _, v := range add {
				addForginService(addr.LogicAddr(v))
			}

			for _, v := range remove {
				removeForginService(addr.LogicAddr(v))
			}

			for _, v := range endPoints {

				if len(add) != 0 {
					AddForginServicesH2S(v, add)
				}

				if len(remove) != 0 {
					RemForginServicesH2S(v, remove)
				}

			}

		}

		replyer.Reply(&NotifyForginServicesH2HResp{}, nil)
	})

	RegisterMethod(&AddForginServicesH2HReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("AddForginServicesH2HReq")
		if isSelfHarbor() {
			msg := req.(*AddForginServicesH2HReq)
			for _, v := range msg.GetNodes() {
				addForginService(addr.LogicAddr(v))
			}
			//向所有非harbor节点通告
			mtx.Lock()
			for tt, v := range ttEndPointMap {
				if tt != harbarType {
					for _, vv := range v.endPoints {
						AddForginServicesH2S(vv, msg.GetNodes())
					}
				}
			}
			mtx.Unlock()
		}
		replyer.Reply(&AddForginServicesH2HResp{}, nil)
	})

	RegisterMethod(&RemForginServicesH2HReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("RemForginServicesH2HReq")
		if isSelfHarbor() {
			msg := req.(*RemForginServicesH2HReq)
			for _, v := range msg.GetNodes() {
				removeForginService(addr.LogicAddr(v))
			}

			//向所有非harbor节点通告
			mtx.Lock()
			for tt, v := range ttEndPointMap {
				if tt != harbarType {
					for _, vv := range v.endPoints {
						RemForginServicesH2S(vv, msg.GetNodes())
					}
				}
			}
			mtx.Unlock()
		}
		replyer.Reply(&RemForginServicesH2HResp{}, nil)

	})
}
