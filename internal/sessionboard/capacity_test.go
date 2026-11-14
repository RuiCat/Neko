// capacity_test.go covers the K3/K9 caps: region eviction, entry trimming,
// board bytes, workspace board count and total store bytes.
package sessionboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRegionEviction covers K3: creating the 65th region evicts the one with
// the oldest updated_at and reports it in evicted.
func TestRegionEviction(t *testing.T) {
	store := newTestStore(t)
	for index := 0; index < MAX_REGIONS; index++ {
		session := "session-" + strconv.Itoa(index)
		mustWrite(t, store, setRequest(t, "/ws/a", session, map[string]string{"k": strconv.Itoa(index)}))
	}
	if read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil); len(read.Board.Regions) != MAX_REGIONS {
		t.Fatalf("regions = %d, want %d", len(read.Board.Regions), MAX_REGIONS)
	}
	result := mustWrite(t, store, setRequest(t, "/ws/a", "session-new", map[string]string{"k": "new"}))
	if len(result.Evicted) != 1 || result.Evicted[0] != "session-0" {
		t.Fatalf("evicted = %v, want [session-0]", result.Evicted)
	}
	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if len(read.Board.Regions) != MAX_REGIONS {
		t.Fatalf("regions = %d, want %d", len(read.Board.Regions), MAX_REGIONS)
	}
	for _, region := range read.Board.Regions {
		if region.ID == "session-0" {
			t.Fatal("最旧区没有被淘汰")
		}
	}
}

// TestCapacity covers the remaining caps: entry trim through the store, the
// 256 KiB board limit, the 128 workspace board limit and the 4 MiB store limit.
func TestCapacity(t *testing.T) {
	t.Run("entry trim", func(t *testing.T) {
		store := newTestStore(t)
		trimmed := []string{}
		write := func(ops []Op) {
			t.Helper()
			result := mustWrite(t, store, WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
				Caller: newCaller("session-a", "/ws/a"), Ops: ops})
			trimmed = append(trimmed, result.Trimmed...)
		}
		for batch := 0; batch < 12; batch++ {
			ops := make([]Op, 0, MAX_OPS)
			for index := 0; index < MAX_OPS; index++ {
				key := "k" + strconv.Itoa(batch*MAX_OPS+index)
				ops = append(ops, Op{Op: OP_SET, Key: key, Value: rawValue(key)})
			}
			write(ops)
		}
		last := make([]Op, 0, 9)
		for index := 192; index < 201; index++ {
			key := "k" + strconv.Itoa(index)
			last = append(last, Op{Op: OP_SET, Key: key, Value: rawValue(key)})
		}
		write(last)
		read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
		if len(read.Board.Regions[0].Entries) != MAX_REGION_ENTRIES {
			t.Fatalf("entries = %d, want %d", len(read.Board.Regions[0].Entries), MAX_REGION_ENTRIES)
		}
		if len(trimmed) != 1 || trimmed[0] != "k0" {
			t.Fatalf("trimmed = %v, want [k0]", trimmed)
		}
	})

	t.Run("board too large", func(t *testing.T) {
		store := newTestStore(t)
		value := strings.Repeat("x", MAX_VALUE_BYTES-2)
		lastVersion := 0
		var code string
		for batch := 0; batch < 10; batch++ {
			ops := make([]Op, 0, MAX_OPS)
			for index := 0; index < MAX_OPS; index++ {
				ops = append(ops, Op{Op: OP_SET, Key: "big-" + strconv.Itoa(batch*MAX_OPS+index),
					Value: rawValue(value)})
			}
			result, err := store.Write(WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
				Caller: newCaller("session-a", "/ws/a"), Ops: ops})
			if err != nil {
				code = errorCodeOf(t, err)
				break
			}
			lastVersion = result.Version
		}
		if code != CODE_BOARD_TOO_LARGE {
			t.Fatalf("code = %q, want %q", code, CODE_BOARD_TOO_LARGE)
		}
		if read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil); read.Board.Version != lastVersion {
			t.Fatalf("version = %d, want %d（超限批不得提交）", read.Board.Version, lastVersion)
		}
	})

	t.Run("workspace limit", func(t *testing.T) {
		store := newTestStore(t)
		for index := 0; index < MAX_WORKSPACE_BOARDS; index++ {
			workspace := "/ws/" + strconv.Itoa(index)
			mustWrite(t, store, setRequest(t, workspace, "session-"+strconv.Itoa(index),
				map[string]string{"k": "v"}))
		}
		_, err := store.Write(setRequest(t, "/ws/overflow", "session-overflow", map[string]string{"k": "v"}))
		if code := errorCodeOf(t, err); code != CODE_WORKSPACE_LIMIT {
			t.Fatalf("code = %q, want %q", code, CODE_WORKSPACE_LIMIT)
		}
	})

	t.Run("store too large", func(t *testing.T) {
		store := newStoreFromLargeGlobal(t)
		_, err := store.Write(setRequest(t, "/ws/new", "session-new", map[string]string{"k": "v"}))
		if code := errorCodeOf(t, err); code != CODE_STORE_TOO_LARGE {
			t.Fatalf("code = %q, want %q", code, CODE_STORE_TOO_LARGE)
		}
	})
}

// newStoreFromLargeGlobal builds a store whose global board alone exceeds the
// 4 MiB store cap (the file is written directly: load does not enforce write
// caps, the write path does).
func newStoreFromLargeGlobal(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), FILE_NAME)
	large := newBoard(SCOPE_GLOBAL, GLOBAL_SCOPE_ID)
	stamp := "2026-11-10T12:00:00+08:00"
	for regionIndex := 0; regionIndex < MAX_REGIONS; regionIndex++ {
		region := Region{ID: "ws-" + strconv.Itoa(regionIndex),
			Owner:   Owner{Kind: OWNER_WORKSPACE, ID: "ws-" + strconv.Itoa(regionIndex)},
			Version: 1, UpdatedAt: stamp, Entries: make([]Entry, 0, MAX_REGION_ENTRIES)}
		for entryIndex := 0; entryIndex < MAX_REGION_ENTRIES; entryIndex++ {
			region.Entries = append(region.Entries, Entry{Key: "k" + strconv.Itoa(entryIndex),
				From: nil, To: strings.Repeat("x", 400), By: "s", Version: 1,
				UpdatedAt: stamp, Deleted: false})
		}
		large.Regions = append(large.Regions, region)
	}
	large.Version = 1
	large.UpdatedAt = stamp
	large.refreshMinRetained()
	raw, err := encodeState(large, map[string]*Board{}, time.Now())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(raw) <= MAX_STORE_BYTES {
		t.Fatalf("测试数据只有 %d 字节，未超过 %d", len(raw), MAX_STORE_BYTES)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return store
}

// TestPersistedShape covers the on-disk shape of K6: schema, global board and
// the workspace map keyed by workspace.
func TestPersistedShape(t *testing.T) {
	store := newTestStore(t)
	mustWrite(t, store, setRequest(t, "/ws/a", "session-a", map[string]string{"a": "1"}))
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	file := map[string]any{}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if file["schema"] != float64(SCHEMA_VERSION) {
		t.Fatalf("schema = %v", file["schema"])
	}
	if _, ok := file["global"].(map[string]any); !ok {
		t.Fatalf("global = %v", file["global"])
	}
	workspace, ok := file["workspace"].(map[string]any)
	if !ok {
		t.Fatalf("workspace = %v", file["workspace"])
	}
	if _, ok := workspace["/ws/a"].(map[string]any); !ok {
		t.Fatalf("workspace map = %v", workspace)
	}
}
