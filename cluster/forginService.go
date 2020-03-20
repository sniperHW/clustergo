/*
 *   非本集群内直连的外部服务，请求需要通过harbor转发
 */

package cluster

import (
	//"fmt"
	//"github.com/kr/pretty"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"math/rand"
	"sort"
)

var ttForignServiceMap map[uint32]*typeForignServiceMap = map[uint32]*typeForignServiceMap{}

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

func addForginService(addr_ addr.LogicAddr) {
	defer mtx.Unlock()
	mtx.Lock()

	m, ok := ttForignServiceMap[addr_.Type()]
	if !ok {
		m = &typeForignServiceMap{
			tt:       addr_.Type(),
			services: []addr.LogicAddr{},
		}
		ttForignServiceMap[addr_.Type()] = m
	}

	m.add(addr_)

	if isSelfHarbor() {
		Infoln("harbor", selfAddr.Logic.String(), "addForginService", addr_.String())
	} else {
		Infoln(selfAddr.Logic.String(), "addForginService", addr_.String())
	}
}

func removeForginService(addr_ addr.LogicAddr) {
	defer mtx.Unlock()
	mtx.Lock()

	m, ok := ttForignServiceMap[addr_.Type()]
	if ok {
		//Infoln("removeForginService", addr_.String())
		m.remove(addr_)
	}
}

func init() {

	RegisterMethod(&NotifyForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("NotifyForginServicesH2SReq") //, pretty.Sprint(req))
		if !isSelfHarbor() {
			msg := req.(*NotifyForginServicesH2SReq)
			mtx.Lock()
			current := []uint32{}
			for _, v1 := range ttForignServiceMap {
				for _, v2 := range v1.services {
					current = append(current, uint32(v2))
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

		}
		replyer.Reply(&AddForginServicesH2SResp{}, nil)
	})

	RegisterMethod(&AddForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("AddForginServicesH2SReq")
		if !isSelfHarbor() {
			msg := req.(*AddForginServicesH2SReq)
			for _, v := range msg.GetNodes() {
				addForginService(addr.LogicAddr(v))
			}
		}
		replyer.Reply(&AddForginServicesH2SResp{}, nil)
	})

	RegisterMethod(&RemForginServicesH2SReq{}, func(replyer *rpc.RPCReplyer, req interface{}) {
		//Infoln("RemForginServicesH2SReq")
		if !isSelfHarbor() {
			msg := req.(*RemForginServicesH2SReq)
			for _, v := range msg.GetNodes() {
				removeForginService(addr.LogicAddr(v))
			}
		}
		replyer.Reply(&RemForginServicesH2SResp{}, nil)
	})
}
