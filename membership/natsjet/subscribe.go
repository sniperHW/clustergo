package natsjet

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/sniperHW/clustergo/membership"
)

// isAlive returns true if the given logic address has an active heartbeat.
func (s *Membership) isAlive(addr string) bool {
	_, ok := s.alive[addr]
	return ok
}

// getMembers performs a full snapshot of the members bucket and emits a single
// MemberInfo describing the diff against s.members.
func (s *Membership) getMembers(ctx context.Context) error {
	keys, err := s.membersKV.Keys()
	if err != nil {
		// ErrNoKeysFound means the bucket is empty; treat as an empty key set so
		// the diff below correctly emits Remove entries for everything previously
		// tracked. Any other error propagates.
		if err != nats.ErrNoKeysFound {
			return GetNatsError(err)
		}
		keys = nil
	}

	currentMembers := make(map[string]*membership.Node, len(keys))
	for _, k := range keys {
		entry, err := s.membersKV.Get(k)
		if err != nil {
			continue
		}
		var n membership.Node
		if err := n.Unmarshal(entry.Value()); err != nil {
			if s.Logger != nil {
				s.Logger.Warnf("natsjet membership unmarshal member failed: %v", err)
			}
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
				Available: false,
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

// getAlives performs a full snapshot of the alive bucket and emits a MemberInfo
// containing Update entries for any node whose alive status flipped.
func (s *Membership) getAlives(ctx context.Context) error {
	keys, err := s.aliveKV.Keys()
	if err != nil {
		// ErrNoKeysFound means the bucket is empty (e.g. after all alive keys
		// expired via TTL); treat as an empty key set so the diff below correctly
		// emits Available=false updates for every node that was alive. Any other
		// error propagates.
		if err != nats.ErrNoKeysFound {
			return GetNatsError(err)
		}
		keys = nil
	}

	currentAlive := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		currentAlive[k] = struct{}{}
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

// handleConfigEntry processes a single entry from the members watcher.
// nil entries mark end-of-snapshot and are ignored.
func (s *Membership) handleConfigEntry(entry nats.KeyValueEntry) {
	if entry == nil {
		return
	}
	var nodeinfo membership.MemberInfo
	logicAddr := entry.Key()

	switch entry.Operation() {
	case nats.KeyValuePut:
		var n membership.Node
		if err := n.Unmarshal(entry.Value()); err != nil {
			if s.Logger != nil {
				s.Logger.Warnf("natsjet membership unmarshal member failed: %v", err)
			}
			return
		}
		logicAddr = n.Addr.LogicAddr().String()
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

	case nats.KeyValueDelete, nats.KeyValuePurge:
		if n, ok := s.members[logicAddr]; ok {
			nodeinfo.Remove = append(nodeinfo.Remove, membership.Node{
				Addr:      n.Addr,
				Export:    n.Export,
				Available: false,
			})
			delete(s.members, logicAddr)
		}
	}

	if s.cb != nil && (len(nodeinfo.Add) > 0 || len(nodeinfo.Update) > 0 || len(nodeinfo.Remove) > 0) {
		s.cb(nodeinfo)
	}
}

// handleAliveEntry processes a single entry from the alive watcher.
// nil entries mark end-of-snapshot and are ignored.
func (s *Membership) handleAliveEntry(entry nats.KeyValueEntry) {
	if entry == nil {
		return
	}
	var nodeinfo membership.MemberInfo
	addr := entry.Key()

	switch entry.Operation() {
	case nats.KeyValuePut:
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
	case nats.KeyValueDelete, nats.KeyValuePurge:
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

	if s.cb != nil && len(nodeinfo.Update) > 0 {
		s.cb(nodeinfo)
	}
}

// watch runs the long-lived subscription loop on two KV watchers with a 5s
// ticker fallback. If a watcher errors, the ticker triggers a full re-sync
// and watcher restart to recover any missed events.
func (s *Membership) watch(ctx context.Context) {
	membersW, err := s.membersKV.WatchAll(nats.Context(ctx))
	if err != nil {
		if s.Logger != nil {
			s.Logger.Errorf("natsjet members watch init failed: %v", err)
		}
	}
	aliveW, err := s.aliveKV.WatchAll(nats.Context(ctx))
	if err != nil {
		if s.Logger != nil {
			s.Logger.Errorf("natsjet alive watch init failed: %v", err)
		}
	}

	var membersCh, aliveCh <-chan nats.KeyValueEntry
	if membersW != nil {
		membersCh = membersW.Updates()
	}
	if aliveW != nil {
		aliveCh = aliveW.Updates()
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	hadError := false

	for {
		select {
		case entry, ok := <-membersCh:
			if !ok {
				return
			}
			s.handleConfigEntry(entry)
		case entry, ok := <-aliveCh:
			if !ok {
				return
			}
			s.handleAliveEntry(entry)
		case <-ticker.C:
			err1 := s.getMembers(ctx)
			err2 := s.getAlives(ctx)
			if err1 != nil || err2 != nil {
				hadError = true
			} else if hadError {
				// Recovery: stop old watchers and open fresh ones.
				if membersW != nil {
					membersW.Stop()
				}
				if aliveW != nil {
					aliveW.Stop()
				}
				membersW, _ = s.membersKV.WatchAll(nats.Context(ctx))
				aliveW, _ = s.aliveKV.WatchAll(nats.Context(ctx))
				if membersW != nil {
					membersCh = membersW.Updates()
				}
				if aliveW != nil {
					aliveCh = aliveW.Updates()
				}
				hadError = false
			}
		case <-ctx.Done():
			if membersW != nil {
				membersW.Stop()
			}
			if aliveW != nil {
				aliveW.Stop()
			}
			return
		}
	}
}

// close stops the watch goroutine and tears down the NATS connection.
// Idempotent via closed CAS.
func (s *Membership) close() {
	if s.closed.CompareAndSwap(false, true) {
		if s.closeFunc != nil {
			s.closeFunc()
		}
		if s.nc != nil {
			s.nc.Close()
		}
	}
}

// Subscribe registers a callback for membership changes and starts a watch
// goroutine. The callback is invoked at least once during Subscribe itself
// with the initial full snapshot. Subsequent invocations happen on the watch
// goroutine. Calling Subscribe twice returns the same close func and ignores
// the second callback.
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
		s.cleanupConn()
		return nil, err
	}
	if err := s.getAlives(ctx); err != nil {
		cancel()
		s.cleanupConn()
		return nil, err
	}

	go s.watch(ctx)
	return s.close, nil
}
