package discovery

import (
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/discovery"
)

type discoverySvr struct {
	nodes map[addr.LogicAddr]*discovery.Node
}

/*
func (d *localDiscovery) LoadNodeInfo() (nodes []discovery.Node, err error) {
	for _, v := range d.nodes {
		nodes = append(nodes, *v)
	}
	return nodes, err
}

// 订阅变更
func (d *localDiscovery) Subscribe(updateCB func([]discovery.Node)) {
	d.subscribes = append(d.subscribes, updateCB)
}*/
