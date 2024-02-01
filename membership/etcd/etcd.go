package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Node struct {
	LogicAddr string
	NetAddr   string
	Export    bool
	Available bool
}

type Client struct {
	members        map[string]*membership.Node //配置中的节点
	alive          map[string]struct{}         //活动节点
	once           sync.Once
	Cfg            clientv3.Config
	PrefixConfig   string
	PrefixAlive    string
	LogicAddr      string
	Logger         clustergo.Logger
	TTL            time.Duration
	rversionConfig int64
	rversionAlive  int64
	cb             func(membership.MemberInfo)
	leaseID        clientv3.LeaseID
	watchConfig    clientv3.WatchChan
	watchAlive     clientv3.WatchChan
	leaseCh        <-chan *clientv3.LeaseKeepAliveResponse
	closeFunc      context.CancelFunc
	closed         atomic.Bool
}

func (ectd *Client) fetchLogicAddr(str string) string {
	v := strings.Split(str, "/")
	if len(v) > 0 {
		return v[len(v)-1]
	} else {
		return ""
	}
}

func (etcd *Client) fetchMembers(ctx context.Context, cli *clientv3.Client) error {
	etcd.members = map[string]*membership.Node{}
	resp, err := cli.Get(ctx, etcd.PrefixConfig, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	for _, v := range resp.Kvs {
		var n Node
		if err := json.Unmarshal(v.Value, &n); err == nil && n.LogicAddr == etcd.fetchLogicAddr(string(v.Key)) {
			if address, err := addr.MakeAddr(n.LogicAddr, n.NetAddr); err == nil {
				nn := membership.Node{
					Addr:      address,
					Available: n.Available,
					Export:    n.Export,
				}
				etcd.members[nn.Addr.LogicAddr().String()] = &nn
			} else {
				etcd.errorf("MakeAddr error,logicAddr:%s,netAddr:%s", n.LogicAddr, n.NetAddr)
			}
		} else if err != nil {
			etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(v.Key), string(v.Value))
		}
	}

	etcd.rversionConfig = resp.Header.GetRevision()
	return nil
}

func (etcd *Client) fetchAlive(ctx context.Context, cli *clientv3.Client) error {
	etcd.alive = map[string]struct{}{}
	resp, err := cli.Get(ctx, etcd.PrefixAlive, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	for _, v := range resp.Kvs {
		key := etcd.fetchLogicAddr(string(v.Key))
		etcd.alive[key] = struct{}{}
		if key == etcd.LogicAddr {
			etcd.leaseID = clientv3.LeaseID(v.Lease)
		}
	}

	etcd.rversionAlive = resp.Header.GetRevision()

	return nil
}

func (etcd *Client) watch(ctx context.Context, cli *clientv3.Client) (err error) {

	if etcd.leaseCh == nil {
		etcd.leaseCh, err = cli.Lease.KeepAlive(ctx, etcd.leaseID)
		if err != nil {
			return err
		}
	}

	if etcd.watchConfig == nil {
		etcd.watchConfig = cli.Watch(ctx, etcd.PrefixConfig, clientv3.WithPrefix(), clientv3.WithRev(etcd.rversionConfig+1))
	}

	if etcd.watchAlive == nil {
		etcd.watchAlive = cli.Watch(ctx, etcd.PrefixAlive, clientv3.WithPrefix(), clientv3.WithRev(etcd.rversionAlive+1))
	}

	for {
		select {
		case _, ok := <-etcd.leaseCh:
			if !ok {
				if respLeaseGrant, err := cli.Lease.Grant(ctx, int64(etcd.TTL/time.Second)); err != nil {
					return err
				} else {
					etcd.leaseID = respLeaseGrant.ID
					_, err = cli.Put(ctx, fmt.Sprintf("%s%s", etcd.PrefixAlive, etcd.LogicAddr), fmt.Sprintf("%x", respLeaseGrant.ID), clientv3.WithLease(respLeaseGrant.ID))
					if err != nil {
						return err
					}
					etcd.leaseCh, err = cli.Lease.KeepAlive(ctx, etcd.leaseID)
					if err != nil {
						return err
					}
				}
			}
		case v := <-etcd.watchConfig:
			if v.Canceled {
				etcd.watchConfig = nil
				return v.Err()
			}
			etcd.rversionConfig = v.Header.GetRevision()
			for _, e := range v.Events {
				var nodeinfo membership.MemberInfo
				key := etcd.fetchLogicAddr(string(e.Kv.Key))
				switch e.Type {
				case clientv3.EventTypePut:
					var n Node
					if err := json.Unmarshal(e.Kv.Value, &n); err == nil && n.LogicAddr == key {
						if address, err := addr.MakeAddr(n.LogicAddr, n.NetAddr); err == nil {
							nn := membership.Node{
								Addr:   address,
								Export: n.Export,
							}

							if _, ok := etcd.alive[key]; ok && n.Available {
								nn.Available = true
							}

							if _, ok := etcd.members[key]; ok {
								nodeinfo.Update = append(nodeinfo.Update, nn)
							} else {
								nodeinfo.Add = append(nodeinfo.Add, nn)
							}

							etcd.members[key] = &nn

							etcd.cb(nodeinfo)
						} else {
							etcd.errorf("MakeAddr error,logicAddr:%s,netAddr:%s", n.LogicAddr, n.NetAddr)
						}
					} else if err != nil {
						etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(e.Kv.Key), string(e.Kv.Value))
					}
				case clientv3.EventTypeDelete:
					if n, ok := etcd.members[key]; ok {
						delete(etcd.members, key)
						nodeinfo.Remove = append(nodeinfo.Remove, *n)
						etcd.cb(nodeinfo)
					}
				}
			}
		case v := <-etcd.watchAlive:
			if v.Canceled {
				etcd.watchAlive = nil
				return v.Err()
			}
			etcd.rversionAlive = v.Header.GetRevision()
			for _, e := range v.Events {
				var nodeinfo membership.MemberInfo
				key := etcd.fetchLogicAddr(string(e.Kv.Key))
				switch e.Type {
				case clientv3.EventTypePut:
					etcd.alive[key] = struct{}{}
					if n, ok := etcd.members[key]; ok && n.Available {
						nodeinfo.Update = append(nodeinfo.Update, *n)
						etcd.cb(nodeinfo)
					}
				case clientv3.EventTypeDelete:
					delete(etcd.alive, key)
					if n, ok := etcd.members[key]; ok {
						n.Available = false
						nodeinfo.Update = append(nodeinfo.Update, *n)
						etcd.cb(nodeinfo)
					}
				}
			}
		}
	}
}

func (etcd *Client) subscribe(ctx context.Context, cli *clientv3.Client) {
	var err error
	if etcd.leaseID == 0 {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Second*5)
		//获取配置
		if err = etcd.fetchMembers(ctxWithTimeout, cli); err != nil {
			cancel()
			etcd.errorf("fetchMembers() error:%v", err)
			return
		}

		//获取alive信息
		if err = etcd.fetchAlive(ctxWithTimeout, cli); err != nil {
			cancel()
			etcd.errorf("fetchAlive() error:%v", err)
			return
		}

		if etcd.leaseID == 0 {
			//创建lease
			if respLeaseGrant, err := cli.Lease.Grant(ctxWithTimeout, int64(etcd.TTL/time.Second)); err != nil {
				cancel()
				etcd.errorf("Lease.Grant() error:%v", err)
				return
			} else {
				etcd.leaseID = respLeaseGrant.ID
				_, err = cli.Put(ctxWithTimeout, fmt.Sprintf("%s%s", etcd.PrefixAlive, etcd.LogicAddr), fmt.Sprintf("%x", etcd.leaseID), clientv3.WithLease(etcd.leaseID))
				if err != nil {
					cancel()
					etcd.errorf("alive put error:%v", err)
					return
				}
			}
		}
		cancel()

		var nodeinfo membership.MemberInfo
		for k, v := range etcd.members {
			if _, ok := etcd.alive[k]; ok && v.Available {
				v.Available = true
			} else {
				v.Available = false
			}
			nodeinfo.Add = append(nodeinfo.Add, *v)
		}

		etcd.cb(nodeinfo)
	}

	if err = etcd.watch(ctx, cli); err != nil {
		etcd.errorf("watch err:%v", err)
	} else {
		log.Println("watch break with no error")
	}
}

func (etcd *Client) Close() {
	if etcd.closed.CompareAndSwap(false, true) {
		if etcd.closeFunc != nil {
			etcd.closeFunc()
		}
	}
}

func (etcd *Client) Subscribe(cb func(membership.MemberInfo)) error {

	once := false

	etcd.once.Do(func() {
		once = true
	})

	if once {
		if etcd.closed.Load() {
			return errors.New("closed")
		}

		etcd.cb = cb
		if etcd.TTL == 0 {
			etcd.TTL = time.Second * 10
		}

		cli, err := clientv3.New(etcd.Cfg)
		if err != nil {
			etcd.errorf("clientv3.New() error:%v", err)
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())

		etcd.closeFunc = cancel

		done := ctx.Done()

		go func() {
			for {
				etcd.subscribe(ctx, cli)
				select {
				case <-done:
					cli.Close()
					return
				default:
					time.Sleep(time.Millisecond * 100)
				}
			}
		}()
	}

	return nil
}

func (etcd *Client) errorf(format string, v ...any) {
	if etcd.Logger != nil {
		etcd.Logger.Errorf(format, v...)
	} else {
		log.Printf(format, v...)
	}
}
