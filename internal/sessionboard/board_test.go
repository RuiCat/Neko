// board_test.go covers the board model: batch application, from/to semantics,
// tombstones, no-net-change, deltas and the per-region entry cap (K1/K3/K4/K5).
package sessionboard

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// testBoard returns an empty workspace board and the pinned clock its writes use.
func testBoard() (Board, time.Time) {
	return newBoard(SCOPE_WORKSPACE, "/ws/a"), time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
}

// setOp builds one validated set operation.
func setOp(key string, value any) writeOp {
	return writeOp{kind: OP_SET, key: key, value: value}
}

// deleteOp builds one validated delete operation.
func deleteOp(key string) writeOp {
	return writeOp{kind: OP_DELETE, key: key}
}

// clearOp builds the single clear operation.
func clearOp() writeOp {
	return writeOp{kind: OP_CLEAR}
}

// entryAt returns one region's entry by key.
func entryAt(t *testing.T, board *Board, key string) Entry {
	t.Helper()
	for _, region := range board.Regions {
		if index := region.entryIndex(key); index >= 0 {
			return region.Entries[index]
		}
	}
	t.Fatalf("找不到条目 %q", key)
	return Entry{}
}

// TestBoardApplyBatch covers one batch writing two keys: one version bump, the
// caller recorded as by, and from null on a first write.
func TestBoardApplyBatch(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	changed, trimmed := board.applyOps(target, "session-a",
		[]writeOp{setOp("任务.当前", "写规划"), setOp("项目.状态", "进行中")}, at)
	if !changed || len(trimmed) != 0 {
		t.Fatalf("applyOps = %v, trimmed=%v", changed, trimmed)
	}
	if board.Version != 1 || board.MinRetainedVersion != 1 {
		t.Fatalf("version=%d min_retained=%d, want 1/1", board.Version, board.MinRetainedVersion)
	}
	if board.UpdatedAt != "2026-11-10T12:00:00+08:00" {
		t.Fatalf("updated_at = %q", board.UpdatedAt)
	}
	if len(board.Regions) != 1 || board.Regions[0].ID != "session-a" {
		t.Fatalf("regions = %+v", board.Regions)
	}
	entry := entryAt(t, &board, "任务.当前")
	if entry.From != nil || entry.To != "写规划" || entry.By != "session-a" ||
		entry.Version != 1 || entry.Deleted || entry.UpdatedAt != board.UpdatedAt {
		t.Fatalf("entry = %+v", entry)
	}
	if board.Regions[0].Version != 1 || board.Regions[0].Owner.Kind != OWNER_SESSION {
		t.Fatalf("region = %+v", board.Regions[0])
	}
}

// TestBoardRewriteAndDelete covers from=previous value on rewrite and the
// tombstone a delete writes (K1).
func TestBoardRewriteAndDelete(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("任务.当前", "写规划")}, at)
	second := at.Add(time.Minute)
	board.applyOps(target, "session-b", []writeOp{setOp("任务.当前", "写实现")}, second)
	entry := entryAt(t, &board, "任务.当前")
	if entry.From != "写规划" || entry.To != "写实现" || entry.By != "session-b" || entry.Version != 2 {
		t.Fatalf("rewrite entry = %+v", entry)
	}
	third := at.Add(2 * time.Minute)
	board.applyOps(target, "session-a", []writeOp{deleteOp("任务.当前")}, third)
	entry = entryAt(t, &board, "任务.当前")
	if !entry.Deleted || entry.To != nil || entry.From != "写实现" || entry.Version != 3 {
		t.Fatalf("tombstone entry = %+v", entry)
	}
	if board.Version != 3 {
		t.Fatalf("version = %d, want 3", board.Version)
	}
}

// TestBoardNoNetChange covers same-value rewrite, delete of a missing key and
// clear of an empty region: none of them is a change (K5).
func TestBoardNoNetChange(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("任务.当前", "写规划")}, at)
	version := board.Version
	changed, _ := board.applyOps(target, "session-a", []writeOp{setOp("任务.当前", "写规划")}, at.Add(time.Minute))
	if changed {
		t.Fatal("同值重写被当成变化")
	}
	changed, _ = board.applyOps(target, "session-a", []writeOp{deleteOp("不存在")}, at.Add(time.Minute))
	if changed {
		t.Fatal("删除不存在键被当成变化")
	}
	empty, emptyAt := newBoard(SCOPE_WORKSPACE, "/ws/a"), at
	changed, _ = empty.applyOps(target, "session-a", []writeOp{clearOp()}, emptyAt)
	if changed || len(empty.Regions) != 0 {
		t.Fatalf("clear 空区 changed=%v regions=%d", changed, len(empty.Regions))
	}
	if board.Version != version {
		t.Fatalf("version = %d, want %d", board.Version, version)
	}
}

// TestBoardClearWritesTombstones covers clear: every live key becomes a
// tombstone and a second clear changes nothing (K5).
func TestBoardClearWritesTombstones(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("a", 1), setOp("b", 2)}, at)
	changed, _ := board.applyOps(target, "session-a", []writeOp{clearOp()}, at.Add(time.Minute))
	if !changed {
		t.Fatal("clear 未报告变化")
	}
	for _, region := range board.Regions {
		for _, entry := range region.Entries {
			if !entry.Deleted || entry.To != nil {
				t.Fatalf("clear 后条目不是 tombstone: %+v", entry)
			}
		}
	}
	changed, _ = board.applyOps(target, "session-a", []writeOp{clearOp()}, at.Add(2*time.Minute))
	if changed {
		t.Fatal("重复 clear 被当成变化")
	}
}

// TestBoardDeltasSince covers the cursor view: entries newer than since, in
// ascending version order, tombstones included (K4).
func TestBoardDeltasSince(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("a", 1)}, at)
	board.applyOps(target, "session-a", []writeOp{setOp("b", 2)}, at.Add(time.Minute))
	board.applyOps(target, "session-a", []writeOp{deleteOp("a")}, at.Add(2*time.Minute))
	deltas := board.deltasSince(1)
	if len(deltas) != 2 {
		t.Fatalf("deltas = %+v", deltas)
	}
	if deltas[0].Key != "b" || deltas[0].Version != 2 || deltas[0].Deleted {
		t.Fatalf("deltas[0] = %+v", deltas[0])
	}
	if deltas[1].Key != "a" || deltas[1].Version != 3 || !deltas[1].Deleted || deltas[1].To != nil {
		t.Fatalf("deltas[1] = %+v", deltas[1])
	}
	if deltas[0].RegionID != "session-a" {
		t.Fatalf("deltas[0].region_id = %q", deltas[0].RegionID)
	}
	if len(board.deltasSince(3)) != 0 {
		t.Fatal("游标已到最新仍有 deltas")
	}
}

// TestBoardLiveViewOmitsTombstones covers the board view: entries only carry
// live keys while tombstones stay in deltas (K4).
func TestBoardLiveViewOmitsTombstones(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("a", 1), setOp("b", 2)}, at)
	board.applyOps(target, "session-a", []writeOp{deleteOp("a")}, at.Add(time.Minute))
	view := board.liveView()
	if len(view.Regions) != 1 || len(view.Regions[0].Entries) != 1 {
		t.Fatalf("live view = %+v", view.Regions)
	}
	if view.Regions[0].Entries[0].Key != "b" {
		t.Fatalf("live entry = %+v", view.Regions[0].Entries[0])
	}
	if view.Version != board.Version || view.MinRetainedVersion != board.MinRetainedVersion {
		t.Fatal("live view 丢了板级字段")
	}
}

// TestBoardTrimEntries covers the 200-entry cap: the oldest entry is dropped,
// listed in trimmed, and the retained watermark moves (K3).
func TestBoardTrimEntries(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	trimmed := []string{}
	for batch := 0; batch < 12; batch++ {
		ops := make([]writeOp, 0, 16)
		for index := 0; index < 16; index++ {
			ops = append(ops, setOp("k"+strconv.Itoa(batch*16+index), index))
		}
		changed, dropped := board.applyOps(target, "session-a", ops, at.Add(time.Duration(batch)*time.Minute))
		if !changed {
			t.Fatalf("batch %d 未变化", batch)
		}
		trimmed = append(trimmed, dropped...)
	}
	last := at.Add(12 * time.Minute)
	ops := make([]writeOp, 0, 9)
	for index := 192; index < 201; index++ {
		ops = append(ops, setOp("k"+strconv.Itoa(index), index))
	}
	changed, dropped := board.applyOps(target, "session-a", ops, last)
	if !changed {
		t.Fatal("最后一批判定无变化")
	}
	trimmed = append(trimmed, dropped...)
	if len(trimmed) != 1 || trimmed[0] != "k0" {
		t.Fatalf("trimmed = %v, want [k0]", trimmed)
	}
	region := board.Regions[0]
	if len(region.Entries) != MAX_REGION_ENTRIES {
		t.Fatalf("entries = %d, want %d", len(region.Entries), MAX_REGION_ENTRIES)
	}
	if region.entryIndex("k0") >= 0 {
		t.Fatal("最旧条目没有被裁剪")
	}
	if board.MinRetainedVersion != region.Entries[0].Version {
		t.Fatalf("min_retained = %d, first retained version = %d",
			board.MinRetainedVersion, region.Entries[0].Version)
	}
}

// TestBoardRegionCap pins the eviction order: the oldest region goes first and
// the newest one survives (K3).
func TestBoardRegionCap(t *testing.T) {
	board, at := testBoard()
	for index := 0; index < MAX_REGIONS; index++ {
		target := Target{RegionID: "session-" + strconv.Itoa(index),
			Owner: Owner{Kind: OWNER_SESSION, ID: "session-" + strconv.Itoa(index)}}
		changed, _ := board.applyOps(target, target.RegionID, []writeOp{setOp("k", index)},
			at.Add(time.Duration(index)*time.Minute))
		if !changed {
			t.Fatalf("第 %d 区未写入", index)
		}
	}
	newest := Target{RegionID: "session-new", Owner: Owner{Kind: OWNER_SESSION, ID: "session-new"}}
	last := at.Add(time.Duration(MAX_REGIONS) * time.Minute)
	board.applyOps(newest, newest.RegionID, []writeOp{setOp("k", "new")}, last)
	evicted := board.enforceRegionCap()
	if len(evicted) != 1 || evicted[0] != "session-0" {
		t.Fatalf("evicted = %v, want [session-0]", evicted)
	}
	if len(board.Regions) != MAX_REGIONS {
		t.Fatalf("regions = %d, want %d", len(board.Regions), MAX_REGIONS)
	}
	if board.regionIndex("session-0") >= 0 || board.regionIndex("session-new") < 0 {
		t.Fatal("淘汰顺序不符：最旧区应被淘汰、新区应保留")
	}
}

// TestBoardCloneIsolation covers the reader copy: mutating a clone never
// touches the stored board.
func TestBoardCloneIsolation(t *testing.T) {
	board, at := testBoard()
	target := Target{RegionID: "session-a", Owner: Owner{Kind: OWNER_SESSION, ID: "session-a"}}
	board.applyOps(target, "session-a", []writeOp{setOp("a", map[string]any{"n": 1})}, at)
	copied := board.clone()
	copied.Regions[0].Entries[0].To.(map[string]any)["n"] = 2
	// Numbers go through JSON and come back as float64, so compare against one.
	if board.Regions[0].Entries[0].To.(map[string]any)["n"] != float64(1) {
		t.Fatal("clone 与源板共享了 JSON 值")
	}
	raw, err := json.Marshal(board)
	if err != nil || len(raw) == 0 {
		t.Fatalf("marshal board: %v", err)
	}
}
