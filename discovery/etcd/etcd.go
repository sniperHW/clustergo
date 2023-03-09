package etcd

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/discovery"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Node struct {
	LogicAddr string
	NetAddr   string
	Export    bool
	Available bool
}

type Discovery struct {
	local  map[string]discovery.Node
	once   sync.Once
	Cfg    clientv3.Config
	Prefix string
	Logger clustergo.Logger
}

func (ectd Discovery) fetchLogicAddr(str string) string {
	if len(str) <= len(ectd.Prefix) {
		return ""
	} else {
		return str[len(ectd.Prefix):]
	}
}

func (etcd *Discovery) subscribe(cb func(discovery.DiscoveryInfo)) {
	for {
		cli, err := clientv3.New(etcd.Cfg)
		if err != nil {
			etcd.errorf("clientv3.New() error:%v", err)
			time.Sleep(time.Millisecond * 100)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			resp, err := cli.Get(ctx, etcd.Prefix, clientv3.WithPrefix())
			cancel()
			if err != nil {
				cli.Close()
				etcd.errorf("cli.Get() error:%v", err)
			} else {
				var nodeinfo discovery.DiscoveryInfo
				for _, v := range resp.Kvs {
					var n Node
					if err := json.Unmarshal(v.Value, &n); err == nil && n.LogicAddr == etcd.fetchLogicAddr(string(v.Key)) {
						if address, err := addr.MakeAddr(n.LogicAddr, n.NetAddr); err == nil {
							nn := discovery.Node{
								Addr:      address,
								Available: n.Available,
								Export:    n.Export,
							}
							etcd.local[nn.Addr.LogicAddr().String()] = nn
							nodeinfo.Add = append(nodeinfo.Add, nn)
						} else {
							etcd.errorf("MakeAddr error,logicAddr:%s,netAddr:%s", n.LogicAddr, n.NetAddr)
						}
					} else if err != nil {
						etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(v.Key), string(v.Value))
					}
				}
				cb(nodeinfo)

				watchCh := cli.Watch(context.Background(), etcd.Prefix, clientv3.WithPrefix(), clientv3.WithRev(resp.Header.GetRevision()))

				for v := range watchCh {
					if v.Canceled {
						break
					}
					var nodeinfo discovery.DiscoveryInfo
					for _, e := range v.Events {
						key := etcd.fetchLogicAddr(string(e.Kv.Key))
						switch e.Type {
						case clientv3.EventTypePut:
							var n Node
							if err := json.Unmarshal(e.Kv.Value, &n); err == nil && n.LogicAddr == key {
								if address, err := addr.MakeAddr(n.LogicAddr, n.NetAddr); err == nil {
									nn := discovery.Node{
										Addr:      address,
										Available: n.Available,
										Export:    n.Export,
									}

									if _, ok := etcd.local[key]; ok {
										nodeinfo.Update = append(nodeinfo.Update, nn)
									} else {
										nodeinfo.Add = append(nodeinfo.Add, nn)
									}
									etcd.local[key] = nn
								} else {
									etcd.errorf("MakeAddr error,logicAddr:%s,netAddr:%s", n.LogicAddr, n.NetAddr)
								}
							} else if err != nil {
								etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(e.Kv.Key), string(e.Kv.Value))
							}
						case clientv3.EventTypeDelete:
							if n, ok := etcd.local[key]; ok {
								delete(etcd.local, key)
								nodeinfo.Remove = append(nodeinfo.Remove, n)
							}
						}
					}
					cb(nodeinfo)
				}
				cli.Close()
			}
		}
	}
}

func (etcd *Discovery) Subscribe(cb func(discovery.DiscoveryInfo)) error {
	etcd.once.Do(func() {
		etcd.local = map[string]discovery.Node{}
		go etcd.subscribe(cb)
	})
	return nil
}

func (etcd *Discovery) errorf(format string, v ...any) {
	if etcd.Logger != nil {
		etcd.Logger.Errorf(format, v...)
	} else {
		log.Printf(format, v...)
	}
}
