package natsjet

import (
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

// UpdateMember inserts or overwrites a member's config in the members bucket.
// Watchers will receive a KeyValuePutOp event.
func (s *Membership) UpdateMember(n membership.Node) error {
	if err := s.init(); err != nil {
		return err
	}
	jsonBytes, err := n.Marshal()
	if err != nil {
		return err
	}
	_, err = s.membersKV.Put(n.Addr.LogicAddr().String(), jsonBytes)
	return GetNatsError(err)
}

// RemoveMember deletes a member by logic address. Idempotent — deleting a
// non-existent key returns nil thanks to GetNatsError.
// Watchers will receive a KeyValueDeleteOp event.
func (s *Membership) RemoveMember(la addr.LogicAddr) error {
	if err := s.init(); err != nil {
		return err
	}
	return GetNatsError(s.membersKV.Delete(la.String()))
}

// KeepAlive records a heartbeat for the given logic address in the alive bucket.
//
// The second int argument is IGNORED — NATS KV TTL is bucket-level, so per-key
// TTL control is impossible on a Put. The bucket TTL is set once via
// Membership.AliveTTL at init() time. Each Put refreshes the key's expiry
// timer, mirroring etcd lease keepalive semantics.
func (s *Membership) KeepAlive(la addr.LogicAddr, _ int) error {
	if err := s.init(); err != nil {
		return err
	}
	_, err := s.aliveKV.Put(la.String(), []byte("1"))
	return GetNatsError(err)
}
