package redis

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

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

// memberCollector collects MemberInfo callbacks for assertions.
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

// waitUntil polls fn every 100ms until it returns true or timeout.
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

// ==================== Unit Tests ====================

func TestGetRedisError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{"nil error", nil, true},
		{"redis nil string", fmt.Errorf("redis: nil"), true},
		{"real error", fmt.Errorf("connection refused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRedisError(tt.err)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetRedisError(%v) = %v, wantNil %v", tt.err, got, tt.wantNil)
			}
		})
	}
}

// ==================== Admin Tests ====================

func TestAdmin_UpdateMember(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	node := membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	}
	if err := m.UpdateMember(node); err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	re, err := getMembers.eval(context.Background(), cli, []string{}, 0)
	if err != nil {
		t.Fatalf("getMembers failed: %v", err)
	}
	r := re.([]interface{})
	if r[0].(int64) != 1 {
		t.Fatalf("expected version 1, got %d", r[0].(int64))
	}
	if len(r[1].([]interface{})) != 1 {
		t.Fatalf("expected 1 node, got %d", len(r[1].([]interface{})))
	}
}

func TestAdmin_UpdateMemberTwice(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})

	re, _ := getMembers.eval(context.Background(), cli, []string{}, 0)
	r := re.([]interface{})
	if r[0].(int64) != 2 {
		t.Fatalf("expected version 2, got %d", r[0].(int64))
	}
	if len(r[1].([]interface{})) != 1 {
		t.Fatalf("expected 1 node (updated in place), got %d", len(r[1].([]interface{})))
	}
}

func TestAdmin_MultipleMembers(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.2", "192.168.1.2:8011"),
		Available: true,
	})
	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.3", "192.168.1.3:8011"),
		Available: true,
	})

	re, _ := getMembers.eval(context.Background(), cli, []string{}, 0)
	r := re.([]interface{})
	if r[0].(int64) != 3 {
		t.Fatalf("expected version 3, got %d", r[0].(int64))
	}
	if len(r[1].([]interface{})) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(r[1].([]interface{})))
	}
}

func TestAdmin_RemoveMember(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	// Initial fetch (version 0) excludes deleted nodes
	re, _ := getMembers.eval(context.Background(), cli, []string{}, 0)
	nodes := re.([]interface{})[1].([]interface{})
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes after remove, got %d", len(nodes))
	}

	// Incremental fetch includes deleted node with markdel=true
	re, _ = getMembers.eval(context.Background(), cli, []string{}, 1)
	nodes = re.([]interface{})[1].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("expected 1 change since v1, got %d", len(nodes))
	}
	if nodes[0].([]interface{})[2].(string) != "true" {
		t.Fatal("expected markdel=true")
	}
}

func TestAdmin_RemoveMemberNonExistent(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("removing non-existent member should not error: %v", err)
	}
}

func TestAdmin_KeepAlive(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	if err := m.KeepAlive(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("KeepAlive failed: %v", err)
	}

	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	r := re.([]interface{})
	if r[0].(int64) != 1 {
		t.Fatalf("expected alive version 1, got %d", r[0].(int64))
	}
	nodes := r[1].([]interface{})
	if len(nodes) != 1 || nodes[0].([]interface{})[0].(string) != "1.1.1" {
		t.Fatal("expected 1.1.1 to be alive")
	}
	if nodes[0].([]interface{})[1].(string) != "false" {
		t.Fatal("expected dead=false")
	}
}

func TestAdmin_KeepAlive_MultipleNodes(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	m.KeepAlive(makeLogicAddr("1.1.1"))
	m.KeepAlive(makeLogicAddr("1.1.2"))

	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	r := re.([]interface{})
	if r[0].(int64) != 2 {
		t.Fatalf("expected version 2, got %d", r[0].(int64))
	}
	nodes := r[1].([]interface{})
	if len(nodes) != 2 {
		t.Fatalf("expected 2 alive nodes, got %d", len(nodes))
	}
}

func TestAdmin_CheckTimeout(t *testing.T) {
	cli := setupTestDB(t)
	m := &Membership{RedisCli: cli}

	// node1: 1s timeout, node2: 300s timeout
	heartbeat.eval(context.Background(), cli, []string{"1.1.1"}, 1)
	heartbeat.eval(context.Background(), cli, []string{"1.1.2"}, 300)

	time.Sleep(2 * time.Second)

	m.CheckTimeout()

	// Initial fetch: only node2 alive
	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	nodes := re.([]interface{})[1].([]interface{})
	if len(nodes) != 1 || nodes[0].([]interface{})[0].(string) != "1.1.2" {
		t.Fatal("only 1.1.2 should be alive after timeout")
	}

	// Incremental fetch: node1 marked dead
	re, _ = getAlives.eval(context.Background(), cli, []string{}, 2)
	nodes = re.([]interface{})[1].([]interface{})
	found := false
	for _, n := range nodes {
		nd := n.([]interface{})
		if nd[0].(string) == "1.1.1" && nd[1].(string) == "true" {
			found = true
		}
	}
	if !found {
		t.Fatal("node1 should be marked dead in incremental fetch")
	}
}

// ==================== Subscribe Tests ====================

func TestSubscribe_EmptyDB(t *testing.T) {
	cli := setupTestDB(t)

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 0 || c.countUpdate() != 0 || c.countRemove() != 0 {
		t.Fatal("expected no notifications on empty DB")
	}
}

func TestSubscribe_InitialMembers(t *testing.T) {
	cli := setupTestDB(t)

	// Pre-populate two members
	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	})
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.2", "192.168.1.2:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// getMembers called during Subscribe returns both as Add
	if c.countAdd() != 2 {
		t.Fatalf("expected 2 initial adds, got %d", c.countAdd())
	}
}

func TestSubscribe_InitialMembersWithAlive(t *testing.T) {
	cli := setupTestDB(t)

	// Pre-populate member + heartbeat
	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	admin.KeepAlive(makeLogicAddr("1.1.1"))

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
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

func TestSubscribe_NewMemberNotification(t *testing.T) {
	cli := setupTestDB(t)

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	waitUntil(t, func() bool { return c.countAdd() >= 1 }, 5*time.Second,
		"timed out waiting for add notification")
}

func TestSubscribe_UpdateNotification(t *testing.T) {
	cli := setupTestDB(t)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// Initial add
	if c.countAdd() != 1 {
		t.Fatalf("expected 1 initial add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	// Update same node with different netAddr
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})

	waitUntil(t, func() bool { return c.countUpdate() >= 1 }, 5*time.Second,
		"timed out waiting for update notification")
}

func TestSubscribe_RemoveNotification(t *testing.T) {
	cli := setupTestDB(t)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	admin.RemoveMember(makeLogicAddr("1.1.1"))

	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove notification")
}

func TestSubscribe_AddAfterRemove(t *testing.T) {
	cli := setupTestDB(t)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// Initial add
	if c.countAdd() != 1 {
		t.Fatalf("expected 1 initial add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	// Remove
	admin.RemoveMember(makeLogicAddr("1.1.1"))
	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove")

	// Add again
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8013"),
		Available: true,
	})
	waitUntil(t, func() bool { return c.countAdd() >= 2 }, 5*time.Second,
		"timed out waiting for re-add")
}

func TestSubscribe_AliveChange(t *testing.T) {
	cli := setupTestDB(t)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	// Initial: member added but not alive
	if c.countAdd() != 1 {
		t.Fatalf("expected 1 add, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	// Heartbeat -> node becomes alive
	admin.KeepAlive(makeLogicAddr("1.1.1"))

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

func TestSubscribe_TimeoutChange(t *testing.T) {
	cli := setupTestDB(t)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	// Heartbeat with short timeout via direct script call
	heartbeat.eval(context.Background(), cli, []string{"1.1.1"}, 1)

	// Wait for alive notification
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

	// Wait for heartbeat to expire
	time.Sleep(2 * time.Second)

	// Trigger timeout check
	admin.CheckTimeout()

	// Wait for dead notification
	waitUntil(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, u := range c.updates {
			if !u.Available {
				return true
			}
		}
		return false
	}, 5*time.Second, "timed out waiting for dead notification")
}

// ==================== Close Tests ====================

func TestMembership_Close(t *testing.T) {
	cli := setupTestDB(t)

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	closeFunc()

	if !sub.closed.Load() {
		t.Fatal("expected closed=true after close")
	}
}

func TestMembership_DoubleClose(t *testing.T) {
	cli := setupTestDB(t)

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Double close should not panic
	closeFunc()
	closeFunc()

	if !sub.closed.Load() {
		t.Fatal("expected closed=true after double close")
	}
}

func TestMembership_CloseStopsNotifications(t *testing.T) {
	cli := setupTestDB(t)

	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Close immediately
	closeFunc()

	// Wait for watch goroutine to exit
	time.Sleep(500 * time.Millisecond)

	admin := &Membership{RedisCli: cli}
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	time.Sleep(1 * time.Second)

	// Should not have received any notifications after close
	if c.countAdd() != 0 {
		t.Fatalf("expected no add notifications after close, got %d", c.countAdd())
	}
}

// ==================== Integration ====================

func TestFullLifecycle(t *testing.T) {
	cli := setupTestDB(t)
	admin := &Membership{RedisCli: cli}

	// 1. Add two members
	node1 := membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	}
	node2 := membership.Node{
		Addr:      makeAddr("1.1.2", "192.168.1.2:8011"),
		Available: true,
	}
	admin.UpdateMember(node1)
	admin.UpdateMember(node2)

	// 2. Subscribe — should get both as initial adds
	sub := &Membership{RedisCli: cli}
	c := newCollector()

	closeFunc, err := sub.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 2 {
		t.Fatalf("expected 2 initial adds, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	// 3. Update node1 (change netAddr)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})
	waitUntil(t, func() bool { return c.countUpdate() >= 1 }, 5*time.Second,
		"timed out waiting for update notification")

	// 4. Remove node2
	admin.RemoveMember(makeLogicAddr("1.1.2"))
	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove notification")

	// 5. Close subscription
	closeFunc()

	if !sub.closed.Load() {
		t.Fatal("expected closed after close()")
	}
}
