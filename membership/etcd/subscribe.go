package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/membership"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Subscribe struct {
	members        map[string]*membership.Node //配置中的节点
	alive          map[string]struct{}         //活动节点
	once           sync.Once
	Cfg            clientv3.Config
	PrefixConfig   string
	PrefixAlive    string
	Logger         clustergo.Logger
	TTL            time.Duration
	rversionConfig int64
	rversionAlive  int64
	cb             func(membership.MemberInfo)
	watchConfig    clientv3.WatchChan
	watchAlive     clientv3.WatchChan
	closeFunc      context.CancelFunc
	closed         atomic.Bool
}

func (ectd *Subscribe) fetchLogicAddr(str string) string {
	v := strings.Split(str, "/")
	if len(v) > 0 {
		return v[len(v)-1]
	} else {
		return ""
	}
}

func (etcd *Subscribe) fetchMembers(ctx context.Context, cli *clientv3.Client) error {
	etcd.members = map[string]*membership.Node{}
	resp, err := cli.Get(ctx, etcd.PrefixConfig, clientv3.WithPrefix())
	if err != nil {
		return err
	}
	for _, v := range resp.Kvs {
		var n membership.Node
		if err := json.Unmarshal(v.Value, &n); err == nil && n.Addr.LogicAddr().String() == etcd.fetchLogicAddr(string(v.Key)) {
			nn := membership.Node{
				Addr:      n.Addr,
				Available: n.Available,
				Export:    n.Export,
			}
			etcd.members[nn.Addr.LogicAddr().String()] = &nn
		} else if err != nil {
			etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(v.Key), string(v.Value))
		}
	}

	etcd.rversionConfig = resp.Header.GetRevision()
	return nil
}

func (etcd *Subscribe) fetchAlive(ctx context.Context, cli *clientv3.Client) error {
	etcd.alive = map[string]struct{}{}
	resp, err := cli.Get(ctx, etcd.PrefixAlive, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	for _, v := range resp.Kvs {
		key := etcd.fetchLogicAddr(string(v.Key))
		etcd.alive[key] = struct{}{}
	}

	etcd.rversionAlive = resp.Header.GetRevision()

	return nil
}

func (etcd *Subscribe) watch(ctx context.Context, cli *clientv3.Client) (err error) {

	/*if etcd.LogicAddr == "" {
		etcd.leaseCh = make(chan *clientv3.LeaseKeepAliveResponse)
	} else {
		if etcd.leaseCh == nil {
			etcd.leaseCh, err = cli.Lease.KeepAlive(ctx, etcd.leaseID)
			if err != nil {
				return err
			}
		}
	}*/

	if etcd.watchConfig == nil {
		etcd.watchConfig = cli.Watch(ctx, etcd.PrefixConfig, clientv3.WithPrefix(), clientv3.WithRev(etcd.rversionConfig+1))
	}

	if etcd.watchAlive == nil {
		etcd.watchAlive = cli.Watch(ctx, etcd.PrefixAlive, clientv3.WithPrefix(), clientv3.WithRev(etcd.rversionAlive+1))
	}

	for {
		select {
		/*case _, ok := <-etcd.leaseCh:
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
		}*/
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
					var n membership.Node
					if err := json.Unmarshal(e.Kv.Value, &n); err == nil && n.Addr.LogicAddr().String() == key {

						nn := membership.Node{
							Addr:   n.Addr,
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

					} else if err != nil {
						etcd.errorf("json.Unmarshal error:%v key:%s value:%s", err, string(e.Kv.Key), string(e.Kv.Value))
					}
				case clientv3.EventTypeDelete:
					if n, ok := etcd.members[key]; ok {
						delete(etcd.members, key)

						nn := membership.Node{
							Addr:   n.Addr,
							Export: n.Export,
						}

						if _, ok := etcd.alive[key]; ok && n.Available {
							nn.Available = true
						}

						nodeinfo.Remove = append(nodeinfo.Remove, nn)
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
						nn := *n
						nn.Available = false
						nodeinfo.Update = append(nodeinfo.Update, nn)
						etcd.cb(nodeinfo)
					}
				}
			}
		}
	}
}

func (etcd *Subscribe) subscribe(ctx context.Context, cli *clientv3.Client) {
	var err error
	if etcd.rversionConfig == 0 {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		//获取配置
		if err = etcd.fetchMembers(ctxWithTimeout, cli); err != nil {
			etcd.errorf("fetchMembers() error:%v", err)
			return
		}

		//获取alive信息
		if err = etcd.fetchAlive(ctxWithTimeout, cli); err != nil {
			etcd.errorf("fetchAlive() error:%v", err)
			return
		}

		/*if etcd.LogicAddr != "" && etcd.leaseID == 0 {
			//创建lease
			if respLeaseGrant, err := cli.Lease.Grant(ctxWithTimeout, int64(etcd.TTL/time.Second)); err != nil {
				etcd.errorf("Lease.Grant() error:%v", err)
				return
			} else {
				etcd.leaseID = respLeaseGrant.ID
				_, err = cli.Put(ctxWithTimeout, fmt.Sprintf("%s%s", etcd.PrefixAlive, etcd.LogicAddr), fmt.Sprintf("%x", etcd.leaseID), clientv3.WithLease(etcd.leaseID))
				if err != nil {
					etcd.errorf("alive put error:%v", err)
					return
				}
			}
		}*/

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

func (etcd *Subscribe) Close() {
	if etcd.closed.CompareAndSwap(false, true) {
		if etcd.closeFunc != nil {
			etcd.closeFunc()
		}
	}
}

func (etcd *Subscribe) Subscribe(cb func(membership.MemberInfo)) error {

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

func (etcd *Subscribe) errorf(format string, v ...any) {
	if etcd.Logger != nil {
		etcd.Logger.Errorf(format, v...)
	} else {
		log.Printf(format, v...)
	}
}
