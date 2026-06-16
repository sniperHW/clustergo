package natsjet

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/membership"
)

// Membership is the NATS JetStream KV implementation of membership.Membership.
//
// Two KV buckets are used:
//   - <Prefix>members: TTL=0, holds Node JSON keyed by logic address.
//   - <Prefix>alive:   TTL=AliveTTL, holds heartbeat markers keyed by logic address.
//
// KeepAlive's int second argument is ignored — TTL is bucket-level and cannot
// be set per-key on a Put. See design spec for details.
type Membership struct {
	// Configuration (caller fills these in before calling any method).
	Servers  string        // e.g. "nats://127.0.0.1:4222"
	Prefix   string        // bucket name prefix, e.g. "clusterA." (default "")
	AliveTTL time.Duration // alive bucket TTL (default 10s if zero)
	Logger   clustergo.Logger

	// Runtime state.
	nc        *nats.Conn
	js        nats.JetStreamContext
	membersKV nats.KeyValue
	aliveKV   nats.KeyValue
	alive     map[string]struct{}
	members   map[string]*membership.Node
	cb        func(membership.MemberInfo)
	once      sync.Once
	initMu    sync.Mutex
	closeFunc context.CancelFunc
	closed    atomic.Bool
}

func (s *Membership) membersBucketName() string {
	return s.Prefix + "members"
}

func (s *Membership) aliveBucketName() string {
	return s.Prefix + "alive"
}

// benignErrs are error values that callers should treat as "nothing went wrong
// from the application's perspective" — missing keys, missing buckets, etc.
var benignErrs = []error{
	nats.ErrKeyNotFound,
	nats.ErrBucketNotFound,
	nats.ErrNoKeysFound,
}

// GetNatsError normalizes errors: returns nil for benign "missing" errors,
// returns the original error otherwise.
func GetNatsError(err error) error {
	if err == nil {
		return nil
	}
	for _, e := range benignErrs {
		if errors.Is(err, e) {
			return nil
		}
	}
	return err
}

// init lazily establishes the NATS connection and ensures both buckets exist.
// It is safe to call from any public method; subsequent calls are no-ops.
func (s *Membership) init() error {
	s.initMu.Lock()
	defer s.initMu.Unlock()

	if s.nc != nil {
		return nil
	}

	nc, err := nats.Connect(s.Servers,
		nats.Name("clustergo-membership"),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return err
	}

	s.nc = nc
	s.js = js

	if err := s.ensureBucket(s.membersBucketName(), 0); err != nil {
		s.cleanupConn()
		return err
	}
	ttl := s.AliveTTL
	if ttl == 0 {
		ttl = 10 * time.Second
	}
	if err := s.ensureBucket(s.aliveBucketName(), ttl); err != nil {
		s.cleanupConn()
		return err
	}

	s.membersKV, err = js.KeyValue(s.membersBucketName())
	if err != nil {
		s.cleanupConn()
		return err
	}
	s.aliveKV, err = js.KeyValue(s.aliveBucketName())
	if err != nil {
		s.cleanupConn()
		return err
	}
	return nil
}

// ensureBucket creates the bucket if absent, or binds to it if it already
// exists. Existing-bucket's TTL/History/Storage win — we do not attempt to
// reconcile configuration drift.
func (s *Membership) ensureBucket(name string, ttl time.Duration) error {
	cfg := &nats.KeyValueConfig{
		Bucket:  name,
		TTL:     ttl,
		Storage: nats.MemoryStorage,
		History: 1,
	}
	_, err := s.js.CreateKeyValue(cfg)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			_, err = s.js.KeyValue(name)
		}
	}
	return err
}

// cleanupConn closes the NATS connection and clears stale state. Used when
// init() fails partway through.
func (s *Membership) cleanupConn() {
	if s.nc != nil {
		s.nc.Close()
		s.nc = nil
		s.js = nil
	}
	s.membersKV = nil
	s.aliveKV = nil
}
