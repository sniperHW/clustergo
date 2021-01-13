/*
 *   非本集群内直连的外部服务，请求需要通过harbor转发
 */

package cluster

import (
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	cluster_proto "github.com/sniperHW/sanguo/cluster/proto"
	"math/rand"
	"sort"
)

type typeForignServiceMap struct {
	tt       uint32
	services []addr.LogicAddr
}

func (this *typeForignServiceMap) sort() {
	sort.Slice(this.services, func(i, j int) bool {
		return uint32(this.services[i]) < uint32(this.services[j])
	})
}

func (this *typeForignServiceMap) remove(addr_ addr.LogicAddr) {
	for i, v := range this.services {
		if addr_ == v {
			this.services[i] = this.services[len(this.services)-1]
			this.services = this.services[:len(this.services)-1]
			this.sort()
			break
		}
	}
}

func (this *typeForignServiceMap) add(addr_ addr.LogicAddr) {

	find := false
	for _, v := range this.services {
		if addr_ == v {
			find = true
			break
		}
	}

	if !find {
		this.services = append(this.services, addr_)
		this.sort()
	}
}

func (this *typeForignServiceMap) mod(num int) (addr.LogicAddr, error) {
	size := len(this.services)
	if size > 0 {
		i := num % size
		return this.services[i], nil
	} else {
		return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
	}
}

func (this *typeForignServiceMap) random() (addr.LogicAddr, error) {
	size := len(this.services)
	if size > 0 {
		i := rand.Int() % size
		return this.services[i], nil
	} else {
		return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
	}
}

func (this *serviceManager) addForginService(addr_ addr.LogicAddr) {
	this.Lock()
	defer this.Unlock()

	m, ok := this.ttForignServiceMap[addr_.Type()]
	if !ok {
		m = &typeForignServiceMap{
			tt:       addr_.Type(),
			services: []addr.LogicAddr{},
		}
		this.ttForignServiceMap[addr_.Type()] = m
	}

	m.add(addr_)

	if this.isSelfHarbor() {
		logger.Infoln("harbor", this.cluster.serverState.selfAddr.Logic.String(), "addForginService", addr_.String())
	} else {
		logger.Infoln(this.cluster.serverState.selfAddr.Logic.String(), "addForginService", addr_.String())
	}
}

func (this *serviceManager) removeForginService(addr_ addr.LogicAddr) {
	this.Lock()
	defer this.Unlock()

	m, ok := this.ttForignServiceMap[addr_.Type()]
	if ok {
		//Infoln("removeForginService", addr_.String())
		m.remove(addr_)
	}
}

func (this *serviceManager) getAllForginService() []uint32 {
	this.RLock()
	defer this.RUnlock()
	current := []uint32{}
	for _, v1 := range this.ttForignServiceMap {
		for _, v2 := range v1.services {
			current = append(current, uint32(v2))
		}
	}
	return current
}

func (this *serviceManager) init() {

	this.cluster.RegisterMethod(&cluster_proto.NotifyForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("NotifyForginServicesH2SReq") //, pretty.Sprint(req))
		if testRPCTimeout && rand.Int()%2 == 0 {
			replyer.DropResponse()
		} else {
			if !this.isSelfHarbor() {
				msg := req.(*cluster_proto.NotifyForginServicesH2SReq)

				current := this.getAllForginService()

				add, remove := diff2(msg.GetNodes(), current)

				for _, v := range add {
					this.addForginService(addr.LogicAddr(v))
				}

				for _, v := range remove {
					this.removeForginService(addr.LogicAddr(v))
				}

			}
			replyer.Reply(&cluster_proto.AddForginServicesH2SResp{}, nil)
		}
	})

	this.cluster.RegisterMethod(&cluster_proto.AddForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("AddForginServicesH2SReq")
		if testRPCTimeout && rand.Int()%2 == 0 {
			replyer.DropResponse()
		} else {
			if !this.isSelfHarbor() {
				msg := req.(*cluster_proto.AddForginServicesH2SReq)
				for _, v := range msg.GetNodes() {
					this.addForginService(addr.LogicAddr(v))
				}
			}
			replyer.Reply(&cluster_proto.AddForginServicesH2SResp{}, nil)
		}
	})

	this.cluster.RegisterMethod(&cluster_proto.RemForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("RemForginServicesH2SReq")
		if testRPCTimeout && rand.Int()%2 == 0 {
			replyer.DropResponse()
		} else {
			if !this.isSelfHarbor() {
				msg := req.(*cluster_proto.RemForginServicesH2SReq)
				for _, v := range msg.GetNodes() {
					this.removeForginService(addr.LogicAddr(v))
				}
			}
			replyer.Reply(&cluster_proto.RemForginServicesH2SResp{}, nil)
		}
	})

	this.initHarbor()
}
