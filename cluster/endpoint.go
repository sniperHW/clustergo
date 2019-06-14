package cluster

import (
	"fmt"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"math/rand"
	"net"
	"sort"
	"sync"

	"github.com/sniperHW/kendynet"
)

type connection struct {
	session kendynet.StreamSession
}

type endPoint struct {
	addr          addr.Addr
	pendingMsg    []interface{} //待发送的消息
	pendingCall   []*rpcCall    //待发起的rpc请求
	dialing       bool
	conn          *connection
	mtx           sync.Mutex
	exportService uint32
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
			end := this.endPoints[i]
			this.endPoints[i] = this.endPoints[len(this.endPoints)-1]
			this.endPoints = this.endPoints[:len(this.endPoints)-1]
			this.sort()
			onEndPointLeave(end)
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

func (this *typeEndPointMap) random() (addr.LogicAddr, error) {
	size := len(this.endPoints)
	if size > 0 {
		i := rand.Int() % size
		return this.endPoints[i].addr.Logic, nil
	} else {
		return addr.LogicAddr(0), fmt.Errorf("no available peer")
	}
}

func addEndPoint(peer *center_proto.NodeInfo) *endPoint {
	defer mtx.Unlock()
	mtx.Lock()

	logicAddr := addr.LogicAddr(peer.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp4", peer.GetNetAddr())

	if nil != err {
		return nil
	}

	peerAddr := addr.Addr{
		Logic: logicAddr,
		Net:   netAddr,
	}

	end := idEndPointMap[addr.LogicAddr(peerAddr.Logic)]
	if nil != end {
		onEndPointJoin(end)
		return end
	}

	end = &endPoint{
		addr:          peerAddr,
		exportService: peer.GetExportService(),
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

	onEndPointJoin(end)
	Infoln("addEndPoint", peerAddr.Logic.String(), end)

	return end
}

func removeEndPoint(peer addr.LogicAddr) {
	defer mtx.Unlock()
	mtx.Lock()
	if end, ok := idEndPointMap[peer]; ok {
		Infoln("remove endPoint", peer.String())
		delete(idEndPointMap, peer)
		if peer.Type() == harbarType {
			removeHarbor(peer)
		}
		ttMap := ttEndPointMap[end.addr.Logic.Type()]

		if nil != ttMap {
			ttMap.removeEndPoint(peer)
		}

		if nil != end.conn {
			end.conn.session.Close("remove endPoint", 0)
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
