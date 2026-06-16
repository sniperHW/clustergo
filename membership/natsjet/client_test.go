package natsjet

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

// ==================== Helpers ====================

func makeAddr(logicAddr, netAddr string) addr.Addr {
	a, _ := addr.MakeAddr(logicAddr, netAddr)
	return a
}

func makeLogicAddr(logicAddr string) addr.LogicAddr {
	a, _ := addr.MakeLogicAddr(logicAddr)
	return a
}

type memberCollector struct {
	mu      sync.Mutex
	adds    []membership.Node
	updates []membership.Node
	removes []membership.Node
}

func newCollector() *memberCollector {
	return &memberCollector{}
}

func (c *memberCollector) callback(info membership.MemberInfo) {
	c.mu.Lock()
	c.adds = append(c.adds, info.Add...)
	c.updates = append(c.updates, info.Update...)
	c.removes = append(c.removes, info.Remove...)
	c.mu.Unlock()
}

func (c *memberCollector) countAdd() int    { c.mu.Lock(); defer c.mu.Unlock(); return len(c.adds) }
func (c *memberCollector) countUpdate() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.updates) }
func (c *memberCollector) countRemove() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.removes) }

func (c *memberCollector) lastAdd() membership.Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.adds) == 0 {
		return membership.Node{}
	}
	return c.adds[len(c.adds)-1]
}

func (c *memberCollector) lastUpdate() membership.Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.updates) == 0 {
		return membership.Node{}
	}
	return c.updates[len(c.updates)-1]
}

func (c *memberCollector) lastRemove() membership.Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.removes) == 0 {
		return membership.Node{}
	}
	return c.removes[len(c.removes)-1]
}

func waitUntil(t *testing.T, fn func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if fn() {
			return
		}
		select {
		case <-deadline:
			t.Fatal(msg)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// setupTestDB connects to local nats-server, drops both buckets so each
// test starts clean, and returns a configured *Membership.
// aliveTTL is used as the alive bucket TTL — pass a small value (e.g. 1s)
// for timeout-related tests.
func setupTestDB(t *testing.T, aliveTTL time.Duration) *Membership {
	t.Helper()
	nc, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatalf("connect nats failed: %v (is membership/natsjet/start.sh running?)", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		t.Fatalf("JetStream context failed: %v", err)
	}
	_ = js.DeleteKeyValue("members")
	_ = js.DeleteKeyValue("alive")
	t.Cleanup(func() {
		_ = js.DeleteKeyValue("members")
		_ = js.DeleteKeyValue("alive")
		nc.Close()
	})
	return &Membership{
		Servers:  "nats://127.0.0.1:4222",
		AliveTTL: aliveTTL,
	}
}

// ==================== Unit Tests ====================

func TestGetNatsError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{"nil error", nil, true},
		{"key not found", nats.ErrKeyNotFound, true},
		{"bucket not found", nats.ErrBucketNotFound, true},
		{"no keys found", nats.ErrNoKeysFound, true},
		{"real error", fmt.Errorf("connection refused"), false},
		{"wrapped key not found", fmt.Errorf("wrap: %w", nats.ErrKeyNotFound), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetNatsError(tt.err)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetNatsError(%v) = %v, wantNil %v", tt.err, got, tt.wantNil)
			}
		})
	}
}

// ==================== Admin Tests ====================

func TestAdmin_UpdateMember(t *testing.T) {
	m := setupTestDB(t, 0)

	node := membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	}
	if err := m.UpdateMember(node); err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	entry, err := m.membersKV.Get("1.1.1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var got membership.Node
	if err := got.Unmarshal(entry.Value()); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.Addr.NetAddr().String() != "192.168.1.1:8011" {
		t.Fatalf("expected 192.168.1.1:8011, got %s", got.Addr.NetAddr().String())
	}
	if !got.Export || !got.Available {
		t.Fatal("expected Export=true, Available=true")
	}
}

func TestAdmin_UpdateMemberTwice(t *testing.T) {
	m := setupTestDB(t, 0)

	if err := m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	}); err != nil {
		t.Fatalf("first UpdateMember: %v", err)
	}
	if err := m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	}); err != nil {
		t.Fatalf("second UpdateMember: %v", err)
	}

	entry, err := m.membersKV.Get("1.1.1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var got membership.Node
	if err := got.Unmarshal(entry.Value()); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.Addr.NetAddr().String() != "192.168.1.1:8012" {
		t.Fatalf("expected update to 192.168.1.1:8012, got %s", got.Addr.NetAddr().String())
	}
}

func TestAdmin_MultipleMembers(t *testing.T) {
	m := setupTestDB(t, 0)

	for _, la := range []string{"1.1.1", "1.1.2", "1.1.3"} {
		if err := m.UpdateMember(membership.Node{
			Addr:      makeAddr(la, "192.168.1.1:8011"),
			Available: true,
		}); err != nil {
			t.Fatalf("UpdateMember %s: %v", la, err)
		}
	}

	keys, err := m.membersKV.Keys()
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
}

func TestAdmin_RemoveMember(t *testing.T) {
	m := setupTestDB(t, 0)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	_, err := m.membersKV.Get("1.1.1")
	if err != nats.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound after remove, got %v", err)
	}
}

func TestAdmin_RemoveMemberNonExistent(t *testing.T) {
	m := setupTestDB(t, 0)

	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("removing non-existent member should not error: %v", err)
	}
}

func TestAdmin_KeepAlive(t *testing.T) {
	m := setupTestDB(t, 10*time.Second)

	if err := m.KeepAlive(makeLogicAddr("1.1.1"), 10); err != nil {
		t.Fatalf("KeepAlive failed: %v", err)
	}

	entry, err := m.aliveKV.Get("1.1.1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(entry.Value()) != "1" {
		t.Fatalf("expected value \"1\", got %q", string(entry.Value()))
	}
}

func TestAdmin_KeepAlive_MultipleNodes(t *testing.T) {
	m := setupTestDB(t, 10*time.Second)

	m.KeepAlive(makeLogicAddr("1.1.1"), 10)
	m.KeepAlive(makeLogicAddr("1.1.2"), 10)

	keys, err := m.aliveKV.Keys()
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 alive keys, got %d", len(keys))
	}
}

// TestAdmin_AliveTTL verifies the alive bucket TTL expires keys automatically.
// Uses a 1s TTL to keep the test fast.
func TestAdmin_AliveTTL(t *testing.T) {
	m := setupTestDB(t, 1*time.Second)

	m.KeepAlive(makeLogicAddr("1.1.1"), 10)

	// Confirm key exists right after Put.
	if _, err := m.aliveKV.Get("1.1.1"); err != nil {
		t.Fatalf("expected key present immediately, got %v", err)
	}

	// TTL is 1s; sleep 2s to be safe (TTL precision in nats-server is approximate).
	time.Sleep(2 * time.Second)

	_, err := m.aliveKV.Get("1.1.1")
	if err != nats.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound after TTL expiry, got %v", err)
	}
}

// ==================== Subscribe Tests (initial snapshot) ====================

func TestSubscribe_EmptyDB(t *testing.T) {
	m := setupTestDB(t, 0)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 0 || c.countUpdate() != 0 || c.countRemove() != 0 {
		t.Fatal("expected no notifications on empty DB")
	}
}

func TestSubscribe_InitialMembers(t *testing.T) {
	m := setupTestDB(t, 0)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	})
	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.2", "192.168.1.2:8011"),
		Available: true,
	})

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 2 {
		t.Fatalf("expected 2 initial adds, got %d", c.countAdd())
	}
}

func TestSubscribe_InitialMembersWithAlive(t *testing.T) {
	m := setupTestDB(t, 10*time.Second)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	m.KeepAlive(makeLogicAddr("1.1.1"), 10)

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// getMembers runs first (alive set empty) → Add with Available=false
	if c.countAdd() != 1 {
		t.Fatalf("expected 1 add, got %d", c.countAdd())
	}

	// getAlives runs second → Update with Available=true
	if c.countUpdate() != 1 {
		t.Fatalf("expected 1 update (alive transition), got %d", c.countUpdate())
	}
	last := c.lastUpdate()
	if !last.Available {
		t.Fatal("expected Available=true in alive update")
	}
}

// ==================== Subscribe Tests (incremental) ====================

func TestSubscribe_NewMemberNotification(t *testing.T) {
	m := setupTestDB(t, 0)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	waitUntil(t, func() bool { return c.countAdd() >= 1 }, 5*time.Second,
		"timed out waiting for add notification")
}

func TestSubscribe_UpdateNotification(t *testing.T) {
	m := setupTestDB(t, 0)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 1 {
		t.Fatalf("expected 1 initial add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})

	waitUntil(t, func() bool { return c.countUpdate() >= 1 }, 5*time.Second,
		"timed out waiting for update notification")
}

func TestSubscribe_RemoveNotification(t *testing.T) {
	m := setupTestDB(t, 0)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	m.RemoveMember(makeLogicAddr("1.1.1"))

	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove notification")
}

func TestSubscribe_AddAfterRemove(t *testing.T) {
	m := setupTestDB(t, 0)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 1 {
		t.Fatalf("expected 1 initial add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	m.RemoveMember(makeLogicAddr("1.1.1"))
	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove")

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8013"),
		Available: true,
	})
	waitUntil(t, func() bool { return c.countAdd() >= 2 }, 5*time.Second,
		"timed out waiting for re-add")
}

func TestSubscribe_AliveChange(t *testing.T) {
	m := setupTestDB(t, 10*time.Second)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 1 {
		t.Fatalf("expected 1 add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	m.KeepAlive(makeLogicAddr("1.1.1"), 10)

	waitUntil(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, u := range c.updates {
			if u.Available {
				return true
			}
		}
		return false
	}, 5*time.Second, "timed out waiting for alive notification")
}

// TestSubscribe_TimeoutChange verifies that an alive key expiring fires an
// Update with Available=false. Uses a 1s TTL; sleeps 2s to clear TTL precision.
func TestSubscribe_TimeoutChange(t *testing.T) {
	m := setupTestDB(t, 1*time.Second)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	m.KeepAlive(makeLogicAddr("1.1.1"), 10)

	c := newCollector()
	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// Wait for initial add + alive update.
	waitUntil(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		seenAlive := false
		for _, u := range c.updates {
			if u.Available {
				seenAlive = true
				break
			}
		}
		return seenAlive
	}, 5*time.Second, "timed out waiting for initial alive notification")

	// TTL=1s, sleep 2s to ensure expiry.
	time.Sleep(2 * time.Second)

	waitUntil(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, u := range c.updates {
			if !u.Available {
				return true
			}
		}
		return false
	}, 5*time.Second, "timed out waiting for timeout notification")
}
