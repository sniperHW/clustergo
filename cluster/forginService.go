/*
 *   非本集群内直连的外部服务，请求需要通过harbor转发
 */

package cluster

import (
	"fmt"
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

func (this *typeForignServiceMap) random() (addr.LogicAddr, error) {
	size := len(this.services)
	if size > 0 {
		i := rand.Int() % size
		return this.services[i], nil
	} else {
		return addr.LogicAddr(0), fmt.Errorf("no available service")
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

	Infoln("addForginService", addr_.String())
}

func removeForginService(addr_ addr.LogicAddr) {
	defer mtx.Unlock()
	mtx.Lock()

	m, ok := ttForignServiceMap[addr_.Type()]
	if ok {
		m.remove(addr_)
	}
}

//非harbor节点执行
func onAddForginServicesH2S(msg *AddForginServicesH2S) {
	if !isSelfHarbor() {
		Infoln("onAddForginServicesH2S", msg.GetNodes())
		for _, v := range msg.GetNodes() {
			addForginService(addr.LogicAddr(v))
		}
	}
}

//非harbor节点执行
func onRemForginServicesH2S(msg *RemForginServicesH2S) {
	if !isSelfHarbor() {
		for _, v := range msg.GetNodes() {
			removeForginService(addr.LogicAddr(v))
		}
	}
}
