# NATS JetStream Membership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `membership/natsjet/` package — a third `membership.Membership` backend backed by NATS JetStream KV.

**Architecture:** Two JetStream KV buckets per cluster: `<Prefix>members` (TTL=0, holds member config JSON) and `<Prefix>alive` (TTL=`AliveTTL`, holds heartbeat markers). Writes use `Put` / `Delete`; reads use `WatchAll` (initial snapshot + incremental events) with a 5s ticker fallback. `KeepAlive`'s `second` parameter is ignored — TTL is bucket-level.

**Tech Stack:** Go 1.21, `github.com/nats-io/nats.go`, `nats-server` for tests, `testify` is NOT used (existing redis/etcd tests use stdlib `testing`).

**Spec:** `docs/superpowers/specs/2026-06-16-nats-jetstream-membership-design.md`

---

## File Structure

| Path | Responsibility |
|---|---|
| `membership/natsjet/nats.go` | `Membership` struct, `init()`, `ensureBucket`, `cleanupConn`, `GetNatsError`, bucket name helpers |
| `membership/natsjet/admin.go` | `UpdateMember`, `RemoveMember`, `KeepAlive` — write side |
| `membership/natsjet/subscribe.go` | `Subscribe`, `getMembers`, `getAlives`, `watch`, `handleConfigEntry`, `handleAliveEntry`, `close` — read side |
| `membership/natsjet/client_test.go` | Integration tests using local `nats://127.0.0.1:4222` |
| `membership/natsjet/start.sh` | Launch single-node nats-server with JetStream |
| `doc/membership.md` | Append `## natsjet 实现` section |
| `go.mod` / `go.sum` | Add `github.com/nats-io/nats.go` dependency |

---

## Prerequisites (do once before Task 1)

- [ ] **P1: Install nats-server**

```bash
# macOS
brew install nats-server
# Verify
nats-server --version
```

If `brew` is unavailable, see https://docs.nats.io/running-a-nats-service/introduction/installation for binary releases.

---

## Task 1: Add nats.go dependency and start.sh

**Files:**
- Create: `membership/natsjet/start.sh`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add nats.go dependency**

Run from repo root:
```bash
go get github.com/nats-io/nats.go@latest
go mod tidy
```

Expected: `go.mod` now contains `github.com/nats-io/nats.go vX.Y.Z` in the `require` block. `go.sum` updated.

- [ ] **Step 2: Verify build still passes**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 3: Create start.sh**

Create `membership/natsjet/start.sh` with executable permissions:

```bash
#!/bin/bash
# 启动单节点 nats-server，开启 JetStream，store-dir=当前目录

DIR="$(cd "$(dirname "$0")" && pwd)"
JSDIR="${DIR}/js"
mkdir -p "${JSDIR}"

cat > "${DIR}/nats-server.conf" <<EOF
jetstream {
  store_dir: "${JSDIR}"
}
EOF

exec nats-server \
  -c "${DIR}/nats-server.conf" \
  -a 127.0.0.1 \
  -p 4222
```

Make it executable:
```bash
chmod +x membership/natsjet/start.sh
```

- [ ] **Step 4: Smoke-test nats-server starts**

In a separate terminal:
```bash
membership/natsjet/start.sh
```

Expected: logs include `Listening for client connections on 127.0.0.1:4222` and `JetStream is enabled`. Press Ctrl+C to stop.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum membership/natsjet/start.sh
git commit -m "chore(membership/natsjet): add nats.go dep and start.sh"
```

---

## Task 2: nats.go — struct, bucket names, GetNatsError

**Files:**
- Create: `membership/natsjet/nats.go`
- Create: `membership/natsjet/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `membership/natsjet/client_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails (no package yet)**

```bash
go test ./membership/natsjet/... -run TestGetNatsError -v
```

Expected: FAIL — `no Go files in membership/natsjet`.

- [ ] **Step 3: Write the implementation**

Create `membership/natsjet/nats.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./membership/natsjet/... -run TestGetNatsError -v
```

Expected: PASS — `TestGetNatsError` and all subtests pass.

- [ ] **Step 5: Commit**

```bash
git add membership/natsjet/nats.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add Membership struct and GetNatsError"
```

---

## Task 3: admin.go — UpdateMember + init() + ensureBucket

This task delivers the first write path; `init()` and `ensureBucket()` are exercised indirectly through `UpdateMember`.

**Files:**
- Create: `membership/natsjet/admin.go`
- Modify: `membership/natsjet/nats.go` — add `init()` / `ensureBucket()` / `cleanupConn()`
- Modify: `membership/natsjet/client_test.go` — add admin tests

- [ ] **Step 1: Add init() / ensureBucket() / cleanupConn() to nats.go**

Append to `membership/natsjet/nats.go`:

```go
// init lazily establishes the NATS connection and ensures both buckets exist.
// It is safe to call from any public method; subsequent calls are no-ops.
func (s *Membership) init() error {
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
```

- [ ] **Step 2: Write the failing tests**

Append to `membership/natsjet/client_test.go` (after `TestGetNatsError`):

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./membership/natsjet/... -run TestAdmin -v
```

Expected: FAIL — `m.UpdateMember undefined (type *Membership has no field or method UpdateMember)`. Also requires nats-server running (start with `membership/natsjet/start.sh` in another terminal).

- [ ] **Step 4: Implement UpdateMember**

Create `membership/natsjet/admin.go`:

```go
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

// RemoveMember placeholder — implemented in Task 4.
// Keeping the stub here so admin.go compiles standalone.
var _ addr.LogicAddr
```

Note: the `var _ addr.LogicAddr` line silences the unused-import error for `addr` until Task 4 adds `RemoveMember`. It will be removed in Task 4.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./membership/natsjet/... -run TestAdmin -v
```

Expected: PASS — all three admin tests pass.

- [ ] **Step 6: Commit**

```bash
git add membership/natsjet/admin.go membership/natsjet/nats.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add UpdateMember with init/ensureBucket"
```

---

## Task 4: admin.go — RemoveMember

**Files:**
- Modify: `membership/natsjet/admin.go`
- Modify: `membership/natsjet/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `membership/natsjet/client_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./membership/natsjet/... -run "TestAdmin_RemoveMember" -v
```

Expected: FAIL — `m.RemoveMember undefined`.

- [ ] **Step 3: Implement RemoveMember**

Edit `membership/natsjet/admin.go`. Replace the entire file with:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./membership/natsjet/... -run "TestAdmin_RemoveMember" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add membership/natsjet/admin.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add RemoveMember"
```

---

## Task 5: admin.go — KeepAlive + AliveTTL test

**Files:**
- Modify: `membership/natsjet/admin.go`
- Modify: `membership/natsjet/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `membership/natsjet/client_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./membership/natsjet/... -run "TestAdmin_KeepAlive|TestAdmin_AliveTTL" -v
```

Expected: FAIL — `m.KeepAlive undefined`.

- [ ] **Step 3: Implement KeepAlive**

Edit `membership/natsjet/admin.go`. Append `KeepAlive`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./membership/natsjet/... -run "TestAdmin_KeepAlive|TestAdmin_AliveTTL" -v
```

Expected: PASS. `TestAdmin_AliveTTL` may take ~2s due to the sleep.

- [ ] **Step 5: Commit**

```bash
git add membership/natsjet/admin.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add KeepAlive (bucket-level TTL)"
```

---

## Task 6: subscribe.go — Subscribe + initial snapshot

This task delivers the read path up to and including initial full sync. The watch goroutine comes in Task 7.

**Files:**
- Create: `membership/natsjet/subscribe.go`
- Modify: `membership/natsjet/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `membership/natsjet/client_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./membership/natsjet/... -run "TestSubscribe_" -v
```

Expected: FAIL — `m.Subscribe undefined`.

- [ ] **Step 3: Implement subscribe.go**

Create `membership/natsjet/subscribe.go`:

```go
package natsjet

import (
	"context"

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
		return GetNatsError(err)
	}

	currentMembers := make(map[string]*membership.Node, len(keys))
	for _, k := range keys {
		entry, err := s.membersKV.Get(k)
		if err != nil {
			continue
		}
		var n membership.Node
		if err := n.Unmarshal(entry.Value()); err != nil {
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
		return GetNatsError(err)
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

// watch is implemented in Task 7. Stub here so Subscribe compiles.
func (s *Membership) watch(ctx context.Context) {
	<-ctx.Done()
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
```

Note: the `watch()` stub blocks on `ctx.Done()` so it does nothing for now. Task 7 will replace it entirely.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./membership/natsjet/... -run "TestSubscribe_" -v
```

Expected: PASS — all three initial-snapshot tests pass.

- [ ] **Step 5: Commit**

```bash
git add membership/natsjet/subscribe.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add Subscribe with initial snapshot"
```

---

## Task 7: subscribe.go — watch goroutine + incremental events

**Files:**
- Modify: `membership/natsjet/subscribe.go`
- Modify: `membership/natsjet/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `membership/natsjet/client_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./membership/natsjet/... -run "TestSubscribe_NewMember|TestSubscribe_Update|TestSubscribe_Remove|TestSubscribe_AddAfter|TestSubscribe_Alive|TestSubscribe_Timeout" -v
```

Expected: FAIL — tests time out at `waitUntil` because the watch stub doesn't actually process events.

- [ ] **Step 3: Implement watch + handleConfigEntry + handleAliveEntry**

Edit `membership/natsjet/subscribe.go`. Replace the entire file with:

```go
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
		return GetNatsError(err)
	}

	currentMembers := make(map[string]*membership.Node, len(keys))
	for _, k := range keys {
		entry, err := s.membersKV.Get(k)
		if err != nil {
			continue
		}
		var n membership.Node
		if err := n.Unmarshal(entry.Value()); err != nil {
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
		return GetNatsError(err)
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
	case nats.KeyValuePutOp:
		var n membership.Node
		if err := n.Unmarshal(entry.Value()); err != nil {
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

	case nats.KeyValueDeleteOp, nats.KeyValuePurgeOp:
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
	case nats.KeyValuePutOp:
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
	case nats.KeyValueDeleteOp, nats.KeyValuePurgeOp:
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./membership/natsjet/... -v
```

Expected: PASS — all tests pass. `TestSubscribe_TimeoutChange` may take ~7s (initial alive wait + 2s TTL sleep + timeout wait).

- [ ] **Step 5: Commit**

```bash
git add membership/natsjet/subscribe.go membership/natsjet/client_test.go
git commit -m "feat(membership/natsjet): add watch goroutine and incremental events"
```

---

## Task 8: doc/membership.md — append natsjet section

**Files:**
- Modify: `doc/membership.md`

- [ ] **Step 1: Append the new section**

Edit `doc/membership.md`. After the existing `## etcd 实现` section (and before `## 自定义实现示例`), insert:

```markdown
## natsjet 实现

**文件**：`membership/natsjet/`

### 数据结构
- 两个 JetStream KV bucket：
  - `<Prefix>members`（默认 `members`）：存储成员配置，TTL=0（永不过期）
  - `<Prefix>alive`（默认 `alive`）：存储心跳，TTL=`AliveTTL`（默认 10s）
- key 都是逻辑地址字符串；value 是 `Node.Marshal()` 后的 JSON

### 工作流程
1. `Subscribe` 时通过 `EnsureBucket` 创建 bucket（如不存在），先 `Keys()` 全量拉取 members 和 alive
2. 启动 `watch` goroutine，对两个 bucket 各开一个 `WatchAll()` 订阅
3. watcher 自动推送初始快照 + 后续增量
4. 每 5 秒 ticker 兜底全量同步，防止事件丢失
5. 错误恢复时重建 watcher

### KeepAlive
- alive bucket 的 TTL 是 **bucket 级别**，由 `AliveTTL` 决定
- 每次 `KeepAlive` 调用都会 `Put` 同一个 key，从而重置其过期计时
- **`KeepAlive(addr, second)` 中的 `second` 参数被忽略**——这是 NATS KV 的固有限制（bucket 级 TTL 无法 per-key 控制）
- 节点停止心跳后，`AliveTTL` 到期 → bucket 自动删除 key → watcher 推送 Delete → 客户端收到 `Update(Available=false)`
- 不需要单独的 `CheckTimeout` 调用

### 与 etcd/redis 的差异
| 维度 | natsjet | etcd | redis |
|---|---|---|---|
| 保活机制 | KV bucket TTL | per-lease TTL | Lua 脚本 deadline |
| `KeepAlive` 的 `second` 参数 | **被忽略** | 实际生效 | 实际生效 |
| 超时清理 | 自动（TTL） | 自动（lease） | 手动 `CheckTimeout()` |
| 变更通知 | KV Watch | Watch | Pub/Sub + Lua version |
```

- [ ] **Step 2: Commit**

```bash
git add doc/membership.md
git commit -m "docs(membership): add natsjet section"
```

---

## Verification

After all tasks complete, run the full verification checklist.

- [ ] **V1: Build**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **V2: Vet**

```bash
go vet ./membership/natsjet/...
```

Expected: no output, exit 0.

- [ ] **V3: Full test suite**

In one terminal:
```bash
membership/natsjet/start.sh
```

In another terminal:
```bash
go test ./membership/natsjet/... -v -count=1
```

Expected: every test passes. Total runtime ~10s (dominated by the two TTL tests).

- [ ] **V4: go.mod is clean**

```bash
go mod tidy
git diff go.mod go.sum
```

Expected: empty diff.

- [ ] **V5: Cross-check that the package implements the interface**

```bash
cat > /tmp/iface_check.go <<'EOF'
package main

import (
	"github.com/sniperHW/clustergo/membership"
	"github.com/sniperHW/clustergo/membership/natsjet"
)

var _ membership.Membership = (*natsjet.Membership)(nil)

func main() {}
EOF
go run /tmp/iface_check.go && echo OK
```

Expected: `OK`.

- [ ] **V6: Cleanup**

```bash
rm /tmp/iface_check.go
```
