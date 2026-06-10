package etcd

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Membership struct {
	Cfg          clientv3.Config
	PrefixConfig string
	PrefixAlive  string
	Logger       clustergo.Logger
	TTL          time.Duration

	cli       *clientv3.Client
	alive     map[string]struct{}
	members   map[string]*membership.Node
	cb        func(membership.MemberInfo)
	once      sync.Once
	closeFunc context.CancelFunc
	closed    atomic.Bool
}

func (s *Membership) init() error {
	if s.cli != nil {
		return nil
	}
	cli, err := clientv3.New(s.Cfg)
	if err != nil {
		return err
	}
	s.cli = cli
	return nil
}

func (s *Membership) isAlive(addr string) bool {
	_, ok := s.alive[addr]
	return ok
}

func (s *Membership) getMembers(ctx context.Context) error {
	resp, err := s.cli.Get(ctx, s.PrefixConfig, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	currentMembers := make(map[string]*membership.Node, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var n membership.Node
		if err := n.Unmarshal(kv.Value); err != nil {
			continue
		}
		currentMembers[n.Addr.LogicAddr().String()] = &n
	}

	var nodeinfo membership.MemberInfo

	for addr, n := range s.members {
		if _, ok := currentMembers[addr]; !ok {
			nodeinfo.Remove = append(nodeinfo.Remove, membership.Node{
				Addr:      n.Addr,
				Export:    n.Export,
				Available: s.isAlive(addr) && n.Available,
			})
		}
	}

	for addr, n := range currentMembers {
		nn := membership.Node{
			Addr:      n.Addr,
			Export:    n.Export,
			Available: s.isAlive(addr) && n.Available,
		}
		if _, ok := s.members[addr]; ok {
			nodeinfo.Update = append(nodeinfo.Update, nn)
		} else {
			nodeinfo.Add = append(nodeinfo.Add, nn)
		}
	}

	s.members = currentMembers

	if s.cb != nil && (len(nodeinfo.Add) > 0 || len(nodeinfo.Update) > 0 || len(nodeinfo.Remove) > 0) {
		s.cb(nodeinfo)
	}
	return nil
}

func (s *Membership) getAlives(ctx context.Context) error {
	resp, err := s.cli.Get(ctx, s.PrefixAlive, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	currentAlive := make(map[string]struct{}, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		currentAlive[string(kv.Key)[len(s.PrefixAlive):]] = struct{}{}
	}

	var nodeinfo membership.MemberInfo

	for addr := range currentAlive {
		if _, ok := s.alive[addr]; !ok {
			if n, ok := s.members[addr]; ok && n.Available {
				nodeinfo.Update = append(nodeinfo.Update, membership.Node{
					Addr:      n.Addr,
					Export:    n.Export,
					Available: true,
				})
			}
		}
	}

	for addr := range s.alive {
		if _, ok := currentAlive[addr]; !ok {
			if n, ok := s.members[addr]; ok {
				nodeinfo.Update = append(nodeinfo.Update, membership.Node{
					Addr:      n.Addr,
					Export:    n.Export,
					Available: false,
				})
			}
		}
	}

	s.alive = currentAlive

	if s.cb != nil && len(nodeinfo.Update) > 0 {
		s.cb(nodeinfo)
	}
	return nil
}

func (s *Membership) watch(ctx context.Context) {
	configWatchCh := s.cli.Watch(ctx, s.PrefixConfig, clientv3.WithPrefix())
	aliveWatchCh := s.cli.Watch(ctx, s.PrefixAlive, clientv3.WithPrefix())

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	hadError := false

	for {
		select {
		case resp, ok := <-configWatchCh:
			if !ok {
				return
			}
			if err := resp.Err(); err != nil {
				hadError = true
				if s.Logger != nil {
					s.Logger.Errorf("config watch error: %v", err)
				}
				continue
			}
			s.handleConfigEvents(resp.Events)
		case resp, ok := <-aliveWatchCh:
			if !ok {
				return
			}
			if err := resp.Err(); err != nil {
				hadError = true
				if s.Logger != nil {
					s.Logger.Errorf("alive watch error: %v", err)
				}
				continue
			}
			s.handleAliveEvents(resp.Events)
		case <-ticker.C:
			err1 := s.getMembers(ctx)
			err2 := s.getAlives(ctx)
			if err1 != nil || err2 != nil {
				hadError = true
			}
			if hadError && err1 == nil && err2 == nil {
				hadError = false
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Membership) handleConfigEvents(events []*clientv3.Event) {
	var nodeinfo membership.MemberInfo

	for _, ev := range events {
		switch ev.Type {
		case clientv3.EventTypePut:
			var n membership.Node
			if err := n.Unmarshal(ev.Kv.Value); err != nil {
				continue
			}
			logicAddr := n.Addr.LogicAddr().String()
			nn := membership.Node{
				Addr:      n.Addr,
				Export:    n.Export,
				Available: s.isAlive(logicAddr) && n.Available,
			}
			if _, ok := s.members[logicAddr]; ok {
				nodeinfo.Update = append(nodeinfo.Update, nn)
			} else {
				nodeinfo.Add = append(nodeinfo.Add, nn)
			}
			s.members[logicAddr] = &n

		case clientv3.EventTypeDelete:
			logicAddr := string(ev.Kv.Key)[len(s.PrefixConfig):]
			if n, ok := s.members[logicAddr]; ok {
				nodeinfo.Remove = append(nodeinfo.Remove, membership.Node{
					Addr:      n.Addr,
					Export:    n.Export,
					Available: false,
				})
				delete(s.members, logicAddr)
			}
		}
	}

	if s.cb != nil && (len(nodeinfo.Add) > 0 || len(nodeinfo.Update) > 0 || len(nodeinfo.Remove) > 0) {
		s.cb(nodeinfo)
	}
}

func (s *Membership) handleAliveEvents(events []*clientv3.Event) {
	var nodeinfo membership.MemberInfo

	for _, ev := range events {
		addr := string(ev.Kv.Key)[len(s.PrefixAlive):]
		switch ev.Type {
		case clientv3.EventTypePut:
			if _, ok := s.alive[addr]; !ok {
				s.alive[addr] = struct{}{}
				if n, ok := s.members[addr]; ok && n.Available {
					nodeinfo.Update = append(nodeinfo.Update, membership.Node{
						Addr:      n.Addr,
						Export:    n.Export,
						Available: true,
					})
				}
			}
		case clientv3.EventTypeDelete:
			if _, ok := s.alive[addr]; ok {
				delete(s.alive, addr)
				if n, ok := s.members[addr]; ok {
					nodeinfo.Update = append(nodeinfo.Update, membership.Node{
						Addr:      n.Addr,
						Export:    n.Export,
						Available: false,
					})
				}
			}
		}
	}

	if s.cb != nil && len(nodeinfo.Update) > 0 {
		s.cb(nodeinfo)
	}
}

func (s *Membership) close() {
	if s.closed.CompareAndSwap(false, true) {
		if s.closeFunc != nil {
			s.closeFunc()
		}
		if s.cli != nil {
			s.cli.Close()
		}
	}
}

func (s *Membership) Subscribe(cb func(membership.MemberInfo)) (func(), error) {
	once := false
	s.once.Do(func() {
		once = true
		s.alive = map[string]struct{}{}
		s.members = map[string]*membership.Node{}
	})

	if !once {
		return s.close, nil
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	s.cb = cb

	ctx, cancel := context.WithCancel(context.Background())
	s.closeFunc = cancel

	if err := s.getMembers(ctx); err != nil {
		cancel()
		s.cli.Close()
		return nil, err
	}
	if err := s.getAlives(ctx); err != nil {
		cancel()
		s.cli.Close()
		return nil, err
	}

	go s.watch(ctx)

	return s.close, nil
}

func (s *Membership) UpdateMember(n membership.Node) error {
	if s.cli == nil {
		if err := s.init(); err != nil {
			return err
		}
	}
	jsonBytes, err := n.Marshal()
	if err != nil {
		return err
	}
	_, err = s.cli.Put(context.Background(), s.PrefixConfig+n.Addr.LogicAddr().String(), string(jsonBytes))
	return err
}

func (s *Membership) RemoveMember(n addr.LogicAddr) error {
	if s.cli == nil {
		if err := s.init(); err != nil {
			return err
		}
	}
	_, err := s.cli.Delete(context.Background(), s.PrefixConfig+n.String())
	return err
}

func (s *Membership) KeepAlive(n addr.LogicAddr, second int) error {
	if s.cli == nil {
		if err := s.init(); err != nil {
			return err
		}
	}
	ctx := context.Background()
	resp, err := s.cli.Grant(ctx, int64(second))
	if err != nil {
		return err
	}
	_, err = s.cli.Put(ctx, s.PrefixAlive+n.String(), "1", clientv3.WithLease(resp.ID))
	return err
}
