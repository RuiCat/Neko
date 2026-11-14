// concurrency_test.go pins C14: concurrent writes are serialized, versions are
// monotonic and no batch is lost or half-applied.
package sessionboard

import (
	"strconv"
	"sync"
	"testing"
)

// TestConcurrentWrites runs 100 concurrent writes into one region and checks
// that every batch got its own version and every key survived (C14/K12).
func TestConcurrentWrites(t *testing.T) {
	store := newTestStore(t)
	const writers = 100
	versions := make([]int, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			result, err := store.Write(WriteRequest{
				Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
				Caller: newCaller("session-a", "/ws/a"),
				Ops: []Op{{Op: OP_SET, Key: "k" + strconv.Itoa(index),
					Value: rawValue("v" + strconv.Itoa(index))}},
			})
			if err != nil {
				t.Errorf("并发写入 %d 失败: %v", index, err)
				return
			}
			if !result.Changed {
				t.Errorf("并发写入 %d 报告无变化", index)
				return
			}
			versions[index] = result.Version
		}(index)
	}
	wait.Wait()
	seen := make(map[int]bool, writers)
	for index, version := range versions {
		if version <= 0 {
			t.Fatalf("写入 %d 没有返回版本", index)
		}
		if seen[version] {
			t.Fatalf("版本 %d 重复发放", version)
		}
		seen[version] = true
	}
	for version := 1; version <= writers; version++ {
		if !seen[version] {
			t.Fatalf("版本 %d 缺失（版本必须是 1..%d）", version, writers)
		}
	}
	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if read.Board.Version != writers {
		t.Fatalf("version = %d, want %d", read.Board.Version, writers)
	}
	region := read.Board.Regions[0]
	if len(region.Entries) != writers {
		t.Fatalf("entries = %d, want %d", len(region.Entries), writers)
	}
	keys := make(map[string]bool, writers)
	for _, entry := range region.Entries {
		keys[entry.Key] = true
	}
	for index := 0; index < writers; index++ {
		if !keys["k"+strconv.Itoa(index)] {
			t.Fatalf("键 k%d 丢失", index)
		}
	}
}
