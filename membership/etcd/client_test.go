package etcd

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
	clientv3 "go.etcd.io/etcd/client/v3"
)

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

func (c *memberCollector) lastUpdate() membership.Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.updates) == 0 {
		return membership.Node{}
	}
	return c.updates[len(c.updates)-1]
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

const testPrefixConfig = "/test/members/"
const testPrefixAlive = "/test/alive/"

func setupTestDB(t *testing.T) *clientv3.Client {
	t.Helper()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: time.Second * 5,
	})
	if err != nil {
		t.Fatalf("connect etcd failed: %v", err)
	}
	ctx := context.Background()
	cli.Delete(ctx, testPrefixConfig, clientv3.WithPrefix())
	cli.Delete(ctx, testPrefixAlive, clientv3.WithPrefix())
	t.Cleanup(func() { cli.Close() })
	return cli
}

func newMembership(cli *clientv3.Client) *Membership {
	return &Membership{
		cli:          cli,
		PrefixConfig: testPrefixConfig,
		PrefixAlive:  testPrefixAlive,
	}
}

// ==================== Admin Tests ====================

func TestAdmin_UpdateMember(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	node := membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	}
	if err := m.UpdateMember(node); err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	resp, err := cli.Get(context.Background(), testPrefixConfig, clientv3.WithPrefix())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected 1 key, got %d", len(resp.Kvs))
	}

	var got membership.Node
	if err := got.Unmarshal(resp.Kvs[0].Value); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.Addr.LogicAddr().String() != "1.1.1" {
		t.Fatalf("expected logicAddr 1.1.1, got %s", got.Addr.LogicAddr().String())
	}
}

func TestAdmin_UpdateMemberTwice(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})

	resp, _ := cli.Get(context.Background(), testPrefixConfig, clientv3.WithPrefix())
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected 1 key (updated in place), got %d", len(resp.Kvs))
	}
	var got membership.Node
	got.Unmarshal(resp.Kvs[0].Value)
	if got.Addr.NetAddr().String() != "192.168.1.1:8012" {
		t.Fatalf("expected netAddr 192.168.1.1:8012, got %s", got.Addr.NetAddr().String())
	}
}

func TestAdmin_MultipleMembers(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	m.UpdateMember(membership.Node{Addr: makeAddr("1.1.1", "192.168.1.1:8011"), Available: true})
	m.UpdateMember(membership.Node{Addr: makeAddr("1.1.2", "192.168.1.2:8011"), Available: true})
	m.UpdateMember(membership.Node{Addr: makeAddr("1.1.3", "192.168.1.3:8011"), Available: true})

	resp, _ := cli.Get(context.Background(), testPrefixConfig, clientv3.WithPrefix())
	if len(resp.Kvs) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(resp.Kvs))
	}
}

func TestAdmin_RemoveMember(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	m.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	resp, _ := cli.Get(context.Background(), testPrefixConfig, clientv3.WithPrefix())
	if len(resp.Kvs) != 0 {
		t.Fatalf("expected 0 keys after remove, got %d", len(resp.Kvs))
	}
}

func TestAdmin_RemoveMemberNonExistent(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	if err := m.RemoveMember(makeLogicAddr("1.1.1")); err != nil {
		t.Fatalf("removing non-existent member should not error: %v", err)
	}
}

func TestAdmin_KeepAlive(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	if err := m.KeepAlive(makeLogicAddr("1.1.1"), 10); err != nil {
		t.Fatalf("KeepAlive failed: %v", err)
	}

	resp, _ := cli.Get(context.Background(), testPrefixAlive, clientv3.WithPrefix())
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected 1 alive key, got %d", len(resp.Kvs))
	}
	key := string(resp.Kvs[0].Key)
	if key != testPrefixAlive+"1.1.1" {
		t.Fatalf("expected key %s, got %s", testPrefixAlive+"1.1.1", key)
	}
}

func TestAdmin_KeepAlive_MultipleNodes(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)

	m.KeepAlive(makeLogicAddr("1.1.1"), 10)
	m.KeepAlive(makeLogicAddr("1.1.2"), 10)

	resp, _ := cli.Get(context.Background(), testPrefixAlive, clientv3.WithPrefix())
	if len(resp.Kvs) != 2 {
		t.Fatalf("expected 2 alive keys, got %d", len(resp.Kvs))
	}
}

// ==================== Subscribe Tests ====================

func TestSubscribe_EmptyDB(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)
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
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Export:    true,
		Available: true,
	})
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.2", "192.168.1.2:8011"),
		Available: true,
	})

	m := newMembership(cli)
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
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})
	admin.KeepAlive(makeLogicAddr("1.1.1"), 10)

	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 1 {
		t.Fatalf("expected 1 add, got %d", c.countAdd())
	}

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

	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	waitUntil(t, func() bool { return c.countAdd() >= 1 }, 5*time.Second,
		"timed out waiting for add notification")
}

func TestSubscribe_UpdateNotification(t *testing.T) {
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	m := newMembership(cli)
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

	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})

	waitUntil(t, func() bool { return c.countUpdate() >= 1 }, 5*time.Second,
		"timed out waiting for update notification")
}

func TestSubscribe_RemoveNotification(t *testing.T) {
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
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

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	m := newMembership(cli)
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

	admin.RemoveMember(makeLogicAddr("1.1.1"))
	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove")

	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8013"),
		Available: true,
	})
	waitUntil(t, func() bool { return c.countAdd() >= 2 }, 5*time.Second,
		"timed out waiting for re-add")
}

func TestSubscribe_AliveChange(t *testing.T) {
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	m := newMembership(cli)
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

	admin.KeepAlive(makeLogicAddr("1.1.1"), 10)

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

func TestSubscribe_LeaseExpiry(t *testing.T) {
	cli := setupTestDB(t)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	time.Sleep(500 * time.Millisecond)

	// KeepAlive with short TTL (2 seconds)
	admin.KeepAlive(makeLogicAddr("1.1.1"), 2)

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

	// Wait for lease to expire (2s TTL + buffer)
	fmt.Println("waiting for lease expiry...")
	time.Sleep(4 * time.Second)

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
	}, 10*time.Second, "timed out waiting for dead notification after lease expiry")
}

// ==================== Close Tests ====================

func TestSubscribe_Close(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	closeFunc()

	if !m.closed.Load() {
		t.Fatal("expected closed=true after close")
	}
}

func TestSubscribe_DoubleClose(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	closeFunc()
	closeFunc()

	if !m.closed.Load() {
		t.Fatal("expected closed=true after double close")
	}
}

func TestSubscribe_CloseStopsNotifications(t *testing.T) {
	cli := setupTestDB(t)
	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	closeFunc()

	time.Sleep(500 * time.Millisecond)

	admin := newMembership(cli)
	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8011"),
		Available: true,
	})

	time.Sleep(1 * time.Second)

	if c.countAdd() != 0 {
		t.Fatalf("expected no add notifications after close, got %d", c.countAdd())
	}
}

// ==================== Integration ====================

func TestFullLifecycle(t *testing.T) {
	cli := setupTestDB(t)
	admin := newMembership(cli)

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

	m := newMembership(cli)
	c := newCollector()

	closeFunc, err := m.Subscribe(c.callback)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer closeFunc()

	if c.countAdd() != 2 {
		t.Fatalf("expected 2 initial adds, got %d", c.countAdd())
	}

	time.Sleep(500 * time.Millisecond)

	admin.UpdateMember(membership.Node{
		Addr:      makeAddr("1.1.1", "192.168.1.1:8012"),
		Available: false,
	})
	waitUntil(t, func() bool { return c.countUpdate() >= 1 }, 5*time.Second,
		"timed out waiting for update notification")

	admin.RemoveMember(makeLogicAddr("1.1.2"))
	waitUntil(t, func() bool { return c.countRemove() >= 1 }, 5*time.Second,
		"timed out waiting for remove notification")

	closeFunc()

	if !m.closed.Load() {
		t.Fatal("expected closed after close()")
	}
}
