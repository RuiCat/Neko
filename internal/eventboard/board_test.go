// board_test.go covers the board model: in-place updates, per-tick version,
// entry cap, timestamps and reader copies.
package eventboard

import (
	"fmt"
	"testing"
	"time"
)

// fixedClock returns a pinned timestamp in the local +08:00 offset K1 shows.
func fixedClock(extraMinutes int) time.Time {
	return time.Date(2026, 11, 6, 16, 20+extraMinutes, 0, 0, time.FixedZone("CST", 8*60*60))
}

// boardChange builds one aggregated change for board-level tests.
func boardChange(path string, from, to any, ruleIDs ...string) Change {
	if ruleIDs == nil {
		ruleIDs = []string{}
	}
	return Change{Path: path, From: from, To: to, RuleIDs: ruleIDs}
}

// TestBoardApplyBumpsVersionOncePerTick covers one tick with two paths: both
// entries appear, and the version grows once for the tick.
func TestBoardApplyBumpsVersionOncePerTick(t *testing.T) {
	board := newBoard("梦梦")
	changed := board.apply([]Change{
		boardChange("心情.当前", "满足", "开心", "r1"),
		boardChange("身体.体力", 90, 92, "r2"),
	}, "tick-1", fixedClock(0))
	if !changed {
		t.Fatal("apply reported no change")
	}
	if board.Version != 1 {
		t.Fatalf("version = %d, want 1", board.Version)
	}
	if board.TickID != "tick-1" {
		t.Fatalf("tick_id = %q, want tick-1", board.TickID)
	}
	if board.UpdatedAt != "2026-11-06T16:20:00+08:00" {
		t.Fatalf("updated_at = %q", board.UpdatedAt)
	}
	if len(board.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(board.Entries))
	}
}

// TestBoardApplyUpdatesEntryInPlace covers a second tick changing an existing
// path: the entry is rewritten, not duplicated, and untouched paths stay.
func TestBoardApplyUpdatesEntryInPlace(t *testing.T) {
	board := newBoard("梦梦")
	board.apply([]Change{boardChange("心情.当前", "满足", "开心", "r1"), boardChange("身体.体力", 90, 92, "r2")}, "tick-1", fixedClock(0))
	board.apply([]Change{boardChange("心情.当前", "开心", "兴奋", "r3")}, "tick-2", fixedClock(1))

	if len(board.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 (one per path)", len(board.Entries))
	}
	mood := board.Entries[0]
	if mood.Path != "心情.当前" || mood.From != "开心" || mood.To != "兴奋" {
		t.Fatalf("心情.当前 = %+v, want 开心 -> 兴奋", mood)
	}
	if len(mood.RuleIDs) != 1 || mood.RuleIDs[0] != "r3" {
		t.Fatalf("rule ids = %v, want [r3]", mood.RuleIDs)
	}
	if mood.UpdatedAt != "2026-11-06T16:21:00+08:00" {
		t.Fatalf("entry updated_at = %q", mood.UpdatedAt)
	}
	if board.Entries[1].Path != "身体.体力" || board.Entries[1].To != 92 {
		t.Fatalf("untouched entry lost: %+v", board.Entries[1])
	}
	if board.Version != 2 || board.TickID != "tick-2" {
		t.Fatalf("version/tick = %d/%q, want 2/tick-2", board.Version, board.TickID)
	}
}

// TestBoardApplyEmptyChangesIsNoOp covers a tick without changes for the
// entity: no entries, no version, no timestamp movement.
func TestBoardApplyEmptyChangesIsNoOp(t *testing.T) {
	board := newBoard("梦梦")
	if board.apply(nil, "tick-1", fixedClock(0)) {
		t.Fatal("empty apply reported a change")
	}
	if board.Version != 0 || len(board.Entries) != 0 || board.UpdatedAt != "" || board.TickID != "" {
		t.Fatalf("board changed on empty apply: %+v", board)
	}
}

// TestBoardApplyCapsEntriesAtOldest covers MAX_ENTRIES: the oldest updated_at
// entry is evicted and the newest change still fits.
func TestBoardApplyCapsEntriesAtOldest(t *testing.T) {
	board := newBoard("梦梦")
	first := make([]Change, 0, MAX_ENTRIES)
	for index := 0; index < MAX_ENTRIES; index++ {
		first = append(first, boardChange(fmt.Sprintf("路径.%03d", index), index, index+1))
	}
	board.apply(first, "tick-1", fixedClock(0))
	board.apply([]Change{boardChange("路径.新增", 0, 1)}, "tick-2", fixedClock(1))

	if len(board.Entries) != MAX_ENTRIES {
		t.Fatalf("entries = %d, want %d", len(board.Entries), MAX_ENTRIES)
	}
	if board.Entries[0].Path != "路径.001" {
		t.Fatalf("oldest entry not evicted: first = %s", board.Entries[0].Path)
	}
	if last := board.Entries[len(board.Entries)-1]; last.Path != "路径.新增" {
		t.Fatalf("newest entry missing: last = %s", last.Path)
	}
	if board.Version != 2 {
		t.Fatalf("version = %d, want 2", board.Version)
	}
}

// TestBoardCloneIsIndependent covers reader copies: mutating a clone (including
// nested values and rule ids) never reaches the live board.
func TestBoardCloneIsIndependent(t *testing.T) {
	board := newBoard("梦梦")
	board.apply([]Change{boardChange("状态.复杂", map[string]any{"值": "旧"}, map[string]any{"值": "新"}, "r1")}, "tick-1", fixedClock(0))

	clone := board.clone()
	clone.Entries[0].To.(map[string]any)["值"] = "被改"
	clone.Entries[0].RuleIDs[0] = "被改"
	if got := board.Entries[0].To.(map[string]any)["值"]; got != "新" {
		t.Fatalf("live board aliased by clone: %v", got)
	}
	if board.Entries[0].RuleIDs[0] != "r1" {
		t.Fatalf("live rule ids aliased by clone: %v", board.Entries[0].RuleIDs)
	}
}
