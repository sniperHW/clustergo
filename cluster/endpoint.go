package cluster

import (
	//"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/timer"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/common"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

type endPoint struct {
	sync.Mutex
	addr          addr.Addr
	pendingMsg    []interface{} //待发送的消息
	pendingCall   []*rpcCall    //待发起的rpc请求
	dialing       bool
	session       kendynet.StreamSession
	exportService uint32
	centers       map[net.Addr]bool
	timer         *timer.Timer
	lastActive    time.Time //上次与endPotin通信的时间（收发均可）
}

func (e *endPoint) isHarbor() bool {
	return e.addr.Logic.Type() == harbarType
}

func (e *endPoint) closeSession(reason string) {
	if session := func() kendynet.StreamSession {
		e.Lock()
		defer e.Unlock()

		if nil == e.session {
			return nil
		}

		if nil != e.timer {
			e.timer.Cancel()
			e.timer = nil
		}

		session := e.session
		e.session = nil

		return session

	}(); nil != session {
		session.Close(reason, 0)
	}
}

func (e *endPoint) onTimerTimeout(t *timer.Timer, _ interface{}) {
	if func() bool {
		e.Lock()
		defer e.Unlock()
		if t != e.timer {
			t.Cancel()
			return false
		}

		if time.Now().Unix()-e.lastActive.Unix() > common.HeartBeat_Timeout {
			return true
		} else {
			return false
		}

	}() {
		e.closeSession("timeout")
	}
}

type typeEndPointMap struct {
	tt        uint32
	endPoints []*endPoint
}

func (this *typeEndPointMap) sort() {
	sort.Slice(this.endPoints, func(i, j int) bool {
		return uint32(this.endPoints[i].addr.Logic) < uint32(this.endPoints[j].addr.Logic)
	})
}

func (this *typeEndPointMap) removeEndPoint(peer addr.LogicAddr) {
	for i, v := range this.endPoints {
		if peer == v.addr.Logic {
			this.endPoints[i] = this.endPoints[len(this.endPoints)-1]
			this.endPoints = this.endPoints[:len(this.endPoints)-1]
			this.sort()
			break
		}
	}
}

func (this *typeEndPointMap) addEndPoint(end *endPoint) {

	find := false
	for _, v := range this.endPoints {
		if end.addr.Logic == v.addr.Logic {
			find = true
			break
		}
	}

	if !find {
		this.endPoints = append(this.endPoints, end)
		this.sort()
	}

}

func (this *typeEndPointMap) mod(num int) (addr.LogicAddr, error) {
	size := len(this.endPoints)
	if size > 0 {
		i := num % size
		return this.endPoints[i].addr.Logic, nil
	} else {
		return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
	}
}

func (this *typeEndPointMap) random() (addr.LogicAddr, error) {
	size := len(this.endPoints)
	if size > 0 {
		i := rand.Int() % size
		return this.endPoints[i].addr.Logic, nil
	} else {
		return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
	}
}

func (this *typeEndPointMap) server(serv uint32) (addr.LogicAddr, error) {
	for _, end := range this.endPoints {
		if end.addr.Logic.Server() == serv {
			return end.addr.Logic, nil
		}
	}
	return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
}

func (this *serviceManager) addEndPoint(centerAddr net.Addr, peer *center_proto.NodeInfo) *endPoint {
	this.Lock()
	defer this.Unlock()

	var end *endPoint

	logicAddr := addr.LogicAddr(peer.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp", peer.GetNetAddr())

	if nil != err {
		return nil
	}

	peerAddr := addr.Addr{
		Logic: logicAddr,
		Net:   netAddr,
	}

	defer func() {
		if nil != end {
			end.centers[centerAddr] = true
			logger.Infoln(this.cluster.serverState.selfAddr.Logic.String(), "addEndPoint", peerAddr.Logic.String(), "from", centerAddr.String(), "exportService", 1 == end.exportService)
			this.onEndPointJoin(end)
		}
	}()

	end = this.idEndPointMap[addr.LogicAddr(peerAddr.Logic)]
	if nil != end {
		if end.addr.Net.String() != netAddr.String() {
			end.closeSession("addEndPoint close old connection")
			end.Lock()
			end.addr.Net = netAddr
			end.Unlock()
		}

		return end
	}

	end = &endPoint{
		addr:          peerAddr,
		exportService: peer.GetExportService(),
		centers:       map[net.Addr]bool{},
	}

	ttMap := this.ttEndPointMap[peerAddr.Logic.Type()]
	if nil == ttMap {
		ttMap = &typeEndPointMap{
			tt:        peerAddr.Logic.Type(),
			endPoints: []*endPoint{},
		}
		this.ttEndPointMap[ttMap.tt] = ttMap
	}

	this.idEndPointMap[peerAddr.Logic] = end
	ttMap.addEndPoint(end)

	if peerAddr.Logic.Type() == harbarType {
		this.addHarbor(end)
	}

	return end
}

func (this *serviceManager) removeEndPoint(centerAddr net.Addr, peer addr.LogicAddr) {
	this.Lock()
	defer this.Unlock()
	if end, ok := this.idEndPointMap[peer]; ok {
		delete(end.centers, centerAddr)
		if 0 == len(end.centers) {
			logger.Infoln("remove endPoint", peer.String())
			delete(this.idEndPointMap, peer)
			if ttMap := this.ttEndPointMap[end.addr.Logic.Type()]; nil != ttMap {
				ttMap.removeEndPoint(peer)
			}

			if end.isHarbor() {
				this.removeHarbor(end)
			}

			this.onEndPointLeave(end)
			end.closeSession("remove endPoint")
		}
	}
}

func (this *serviceManager) getEndPoint(id addr.LogicAddr) *endPoint {
	this.RLock()
	defer this.RUnlock()

	if end, ok := this.idEndPointMap[id]; ok {
		return end
	}
	return nil
}

func (this *serviceManager) getAllNodeInfo() []*center_proto.NodeInfo {
	this.RLock()
	defer this.RUnlock()

	currentEndPoints := []*center_proto.NodeInfo{}

	for k, _ := range this.idEndPointMap {
		currentEndPoints = append(currentEndPoints, &center_proto.NodeInfo{
			LogicAddr: proto.Uint32(uint32(k)),
		})
	}

	return currentEndPoints
}

func (this *serviceManager) getAllEndpoints() []*endPoint {
	this.RLock()
	defer this.RUnlock()
	endPoints := []*endPoint{}

	for tt, v := range this.ttEndPointMap {
		if tt != harbarType {
			for _, vv := range v.endPoints {
				endPoints = append(endPoints, vv)
			}
		}
	}

	return endPoints
}
