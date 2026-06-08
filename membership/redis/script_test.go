package redis

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupTestDB(t *testing.T) *redis.Client {
	t.Helper()
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll(context.Background())
	t.Cleanup(func() { cli.Close() })
	return cli
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if GetRedisError(err) != nil {
		t.Fatal(err)
	}
}

func TestScriptMembers_InsertAndGet(t *testing.T) {
	cli := setupTestDB(t)

	// Empty db
	re, err := getMembers.eval(context.Background(), cli, []string{}, 0)
	checkErr(t, err)
	if re.([]interface{})[0].(int64) != 0 {
		t.Fatal("expected version 0 for empty db")
	}

	// Insert two nodes
	_, err = updateMember.eval(context.Background(), cli, []string{"node1"}, "insert_update", "node1_data")
	checkErr(t, err)
	_, err = updateMember.eval(context.Background(), cli, []string{"node2"}, "insert_update", "node2_data")
	checkErr(t, err)

	// Fetch all (version 0) returns both
	re, err = getMembers.eval(context.Background(), cli, []string{}, 0)
	checkErr(t, err)
	result := re.([]interface{})
	if result[0].(int64) != 2 {
		t.Fatalf("expected version 2, got %d", result[0].(int64))
	}
	nodes := result[1].([]interface{})
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Version match returns only version
	re, _ = getMembers.eval(context.Background(), cli, []string{}, 2)
	if len(re.([]interface{})) != 1 {
		t.Fatal("expected only version when client is up-to-date")
	}
}

func TestScriptMembers_Delete(t *testing.T) {
	cli := setupTestDB(t)

	// Insert then delete
	updateMember.eval(context.Background(), cli, []string{"node1"}, "insert_update", "node1_data")
	updateMember.eval(context.Background(), cli, []string{"node1"}, "delete")

	// Initial fetch (version 0) excludes deleted nodes
	re, _ := getMembers.eval(context.Background(), cli, []string{}, 0)
	if len(re.([]interface{})[1].([]interface{})) != 0 {
		t.Fatal("initial fetch should exclude deleted nodes")
	}

	// Incremental fetch returns deleted node with markdel=true
	re, _ = getMembers.eval(context.Background(), cli, []string{}, 1)
	node := re.([]interface{})[1].([]interface{})[0].([]interface{})
	if node[0].(string) != "node1" {
		t.Fatalf("expected addr node1, got %s", node[0])
	}
	if node[1].(string) != "node1_data" {
		t.Fatalf("expected info preserved, got %s", node[1])
	}
	if node[2].(string) != "true" {
		t.Fatalf("expected markdel true, got %s", node[2])
	}

	// Delete non-existent node is no-op
	_, err := updateMember.eval(context.Background(), cli, []string{"nonexist"}, "delete")
	checkErr(t, err)
}

func TestScriptMembers_Update(t *testing.T) {
	cli := setupTestDB(t)

	updateMember.eval(context.Background(), cli, []string{"node1"}, "insert_update", "data_v1")
	updateMember.eval(context.Background(), cli, []string{"node1"}, "insert_update", "data_v2")

	re, _ := getMembers.eval(context.Background(), cli, []string{}, 0)
	node := re.([]interface{})[1].([]interface{})[0].([]interface{})
	if node[1].(string) != "data_v2" {
		t.Fatalf("expected data_v2, got %s", node[1])
	}
}

func TestScriptMembers_IncrementalFetch(t *testing.T) {
	cli := setupTestDB(t)

	// Insert node1 (v1), node2 (v2), delete node1 (v3)
	updateMember.eval(context.Background(), cli, []string{"node1"}, "insert_update", "node1_data")
	updateMember.eval(context.Background(), cli, []string{"node2"}, "insert_update", "node2_data")
	updateMember.eval(context.Background(), cli, []string{"node1"}, "delete")

	// Since v1: node2 added + node1 deleted
	re, _ := getMembers.eval(context.Background(), cli, []string{}, 1)
	nodes := re.([]interface{})[1].([]interface{})
	if len(nodes) != 2 {
		t.Fatalf("expected 2 changes since v1, got %d", len(nodes))
	}

	// Since v2: only node1 deleted
	re, _ = getMembers.eval(context.Background(), cli, []string{}, 2)
	nodes = re.([]interface{})[1].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("expected 1 change since v2, got %d", len(nodes))
	}
	if nodes[0].([]interface{})[2].(string) != "true" {
		t.Fatal("expected node1 markdel=true")
	}
}

func TestScriptAlive_HeartbeatAndGet(t *testing.T) {
	cli := setupTestDB(t)

	// Empty db
	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	if re.([]interface{})[0].(int64) != 0 {
		t.Fatal("expected version 0 for empty db")
	}

	// Heartbeat two nodes
	heartbeat.eval(context.Background(), cli, []string{"node1"}, 10)
	heartbeat.eval(context.Background(), cli, []string{"node2"}, 10)

	// Fetch all alive
	re, _ = getAlives.eval(context.Background(), cli, []string{}, 0)
	result := re.([]interface{})
	if result[0].(int64) != 2 {
		t.Fatalf("expected version 2, got %d", result[0].(int64))
	}
	nodes := result[1].([]interface{})
	if len(nodes) != 2 {
		t.Fatalf("expected 2 alive nodes, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.([]interface{})[1].(string) != "false" {
			t.Fatal("expected dead=false")
		}
	}

	// Version match
	re, _ = getAlives.eval(context.Background(), cli, []string{}, 2)
	if len(re.([]interface{})) != 1 {
		t.Fatal("expected only version when client is up-to-date")
	}
}

func TestScriptCheckTimeout(t *testing.T) {
	cli := setupTestDB(t)

	// node1: 1s timeout, node2: 300s timeout
	heartbeat.eval(context.Background(), cli, []string{"node1"}, 1)
	heartbeat.eval(context.Background(), cli, []string{"node2"}, 300)

	time.Sleep(time.Second * 2)

	checkTimeout.eval(context.Background(), cli, []string{})

	// Initial fetch: only node2 alive
	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	nodes := re.([]interface{})[1].([]interface{})
	if len(nodes) != 1 || nodes[0].([]interface{})[0].(string) != "node2" {
		t.Fatal("only node2 should be alive after timeout")
	}

	// Incremental fetch: node1 marked dead
	re, _ = getAlives.eval(context.Background(), cli, []string{}, 2)
	nodes = re.([]interface{})[1].([]interface{})
	found := false
	for _, n := range nodes {
		nd := n.([]interface{})
		if nd[0].(string) == "node1" && nd[1].(string) == "true" {
			found = true
		}
	}
	if !found {
		t.Fatal("node1 should be marked dead in incremental fetch")
	}
}

func TestScriptAlive_DeadNodeHeartbeat(t *testing.T) {
	cli := setupTestDB(t)

	heartbeat.eval(context.Background(), cli, []string{"node1"}, 1)
	time.Sleep(time.Second * 2)
	checkTimeout.eval(context.Background(), cli, []string{})

	// Record version after death
	re, _ := getAlives.eval(context.Background(), cli, []string{}, 0)
	deadVersion := re.([]interface{})[0].(int64)

	// Dead node sends heartbeat - should NOT bump version
	heartbeat.eval(context.Background(), cli, []string{"node1"}, 300)

	// Version unchanged, no new changes
	re, _ = getAlives.eval(context.Background(), cli, []string{}, deadVersion)
	if len(re.([]interface{})) != 1 {
		t.Fatal("dead node heartbeat should not produce version changes")
	}
}
