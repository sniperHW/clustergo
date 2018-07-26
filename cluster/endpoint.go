package cluster

import(
	"github.com/sniperHW/kendynet"
	"github.com/golang/protobuf/proto"
	_ "sanguo/protocol/ss" //触发pb注册
	"fmt"
	"math/rand"
	"sync"
)

type ttMap  map[string]*endPoint

var (
	idEndPointMap    map[PeerID]*endPoint
	ttEndPointMap 	 map[string]ttMap
	mtx sync.Mutex
	sessionPeerIDMap map[kendynet.StreamSession] PeerID
)

type endPoint struct {
	tt         	  string
	ip         	  string
	port       	  int32
	pendingMsg    []proto.Message      //待发送的消息
	pendingCall   []*rpcCall           //待发起的rpc请求
	dialing       bool
	conn      	 *connection
}

func addEndPoint(end *endPoint) {
	if _,ok := idEndPointMap[end.toPeerID()]; !ok {
		idEndPointMap[end.toPeerID()] = end
		var ttmap ttMap
		if ttmap,ok = ttEndPointMap[end.tt];!ok {
			ttmap = make(ttMap)
			ttEndPointMap[end.tt] = ttmap
		}

		key := fmt.Sprintf("%d%s",len(ttmap),end.tt)
		ttmap[key] = end
		Infof("NotifyNodeInfo %s\n",end.toPeerID())
	}
}

func remEndPoint(peer PeerID) {
	if end,ok := idEndPointMap[peer]; ok {
		delete(idEndPointMap,peer)
		if ttmap,ok := ttEndPointMap[end.tt]; ok {
			tmp := make(ttMap)
			ttEndPointMap[end.tt] = tmp
			for _,v := range(ttmap) {
				if v != end {
					key := fmt.Sprintf("%d%s",len(tmp),v.tt)
					ttmap[key] = v
				}
			}
		}
		if nil != end.conn {
			end.conn.session.Close("Center NodeLose",0)
		}	
		Infof("NodeLose %s\n",peer.ToString())
	}	
}

//随机获取一个类型为tt的节点id
func Random(tt string) (PeerID,error) {
	if ttmap,ok := ttEndPointMap[tt]; ok {
		size := len(ttmap)
		if size > 0 {
			i := rand.Int()%size
			key := fmt.Sprintf("%d%s",i,tt)
			return ttmap[key].toPeerID(),nil
		}
	}
	return MakePeerID("","",0),fmt.Errorf("invaild tt")
}

func (this *endPoint) toPeerID() PeerID {
	return PeerID(fmt.Sprintf("%s@%s:%d",this.tt,this.ip,this.port))
}

func getEndPointByID(id PeerID) *endPoint {
	if end,ok := idEndPointMap[id]; ok {
		return end
	}
	return nil
}


func addSessionPeerID(session kendynet.StreamSession,peerID PeerID) {
	defer mtx.Unlock()
	mtx.Lock()	
	sessionPeerIDMap[session] = peerID
}

func remSessionPeerID(session kendynet.StreamSession) {
	defer mtx.Unlock()
	mtx.Lock()	
	delete(sessionPeerIDMap,session)	
}

func GetPeerIDBySession(session kendynet.StreamSession) (PeerID,error) {
	defer mtx.Unlock()
	mtx.Lock()

	if p,ok := sessionPeerIDMap[session]; ok {
		return p,nil
	}
	return MakePeerID("","",0),fmt.Errorf("unknow session")
}