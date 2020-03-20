package cluster

import (
	//"fmt"
	"github.com/sniperHW/kendynet"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"math/rand"
	"net"
	"sort"
	"sync"
)

type endPoint struct {
	addr          addr.Addr
	pendingMsg    []interface{} //待发送的消息
	pendingCall   []*rpcCall    //待发起的rpc请求
	dialing       bool
	session       kendynet.StreamSession
	mtx           sync.Mutex
	exportService uint32
	centers       map[net.Addr]bool
}

func (e *endPoint) isHarbor() bool {
	return e.addr.Logic.Type() == harbarType
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

func addEndPoint(centerAddr net.Addr, peer *center_proto.NodeInfo) *endPoint {
	defer mtx.Unlock()
	mtx.Lock()

	var end *endPoint

	logicAddr := addr.LogicAddr(peer.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp4", peer.GetNetAddr())

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
			Infoln("addEndPoint", peerAddr.Logic.String(), "from", centerAddr.String(), end.exportService)
			onEndPointJoin(end)
		}
	}()

	end = idEndPointMap[addr.LogicAddr(peerAddr.Logic)]
	if nil != end {

		if end.addr.Net.String() != netAddr.String() {
			end.mtx.Lock()
			oldSession := end.session
			end.session = nil
			end.addr.Net = netAddr //更新网络地址
			end.mtx.Unlock()
			if nil != oldSession {
				//中断原有连接
				oldSession.Close("addEndPoint close old connection", 0)
			}
		}

		return end
	}

	end = &endPoint{
		addr:          peerAddr,
		exportService: peer.GetExportService(),
		centers:       map[net.Addr]bool{},
	}

	ttMap := ttEndPointMap[peerAddr.Logic.Type()]
	if nil == ttMap {
		ttMap = &typeEndPointMap{
			tt:        peerAddr.Logic.Type(),
			endPoints: []*endPoint{},
		}
		ttEndPointMap[ttMap.tt] = ttMap
	}

	idEndPointMap[peerAddr.Logic] = end
	ttMap.addEndPoint(end)

	if peerAddr.Logic.Type() == harbarType {
		addHarbor(end)
	}

	return end
}

func removeEndPoint(centerAddr net.Addr, peer addr.LogicAddr) {
	defer mtx.Unlock()
	mtx.Lock()
	if end, ok := idEndPointMap[peer]; ok {
		Infoln("remove endPoint", peer.String(), "from", centerAddr.String())
		delete(end.centers, centerAddr)
		if 0 == len(end.centers) {
			Infoln("remove endPoint", peer.String())
			delete(idEndPointMap, peer)
			if ttMap := ttEndPointMap[end.addr.Logic.Type()]; nil != ttMap {
				ttMap.removeEndPoint(peer)
			}

			if end.isHarbor() {
				removeHarbor(end)
			}

			onEndPointLeave(end)

			if nil != end.session {
				end.session.Close("remove endPoint", 0)
			}
		}
	}
}

func getEndPoint(id addr.LogicAddr) *endPoint {
	defer mtx.Unlock()
	mtx.Lock()
	if end, ok := idEndPointMap[id]; ok {
		return end
	}
	return nil
}
