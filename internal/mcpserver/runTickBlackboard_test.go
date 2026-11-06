// runTickBlackboard_test.go covers the tick -> change board wiring (K2):
// aggregation, persistence with the tick and failure isolation.
package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/eventboard"
)

// newBoardFixture loads a fresh shared board registry into the tick fixture.
func newBoardFixture(t *testing.T) (*tickFixture, *eventboard.Registry, string) {
	t.Helper()
	fx := newTickFixture(t)
	path := filepath.Join(fx.dataDir, eventboard.FILE_NAME)
	boards := eventboard.New(path)
	if err := boards.Load(); err != nil {
		t.Fatalf("load boards: %v", err)
	}
	fx.server.SetEventBoards(boards)
	return fx, boards, path
}

// addStateRule installs one enabled rule that sets a dotted state path.
func addStateRule(fx *tickFixture, id, path string, value any) {
	fx.machine.AddRule(engine.Rule{
		ID: id, Match: "[*]", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: path, Value: value}}}},
	})
}

// TestRunTickWritesEventBoard covers one changed tick: the board records the
// transition (from/to/rule id), the version moves once and the file lands with
// the tick.
func TestRunTickWritesEventBoard(t *testing.T) {
	fx, boards, path := newBoardFixture(t)
	if err := fx.entity.Set("状态.身体", "健康"); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	addStateRule(fx, "r_board", "状态.身体", "疲惫")

	result, err := fx.server.RunTick("", TickOptions{})
	if err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(result.Transitions))
	}
	board := boards.Board("猫猫")
	if board == nil {
		t.Fatal("board missing after a changed tick")
	}
	if board.Version != 1 || board.TickID != result.TickID {
		t.Fatalf("version/tick = %d/%q, want 1/%q", board.Version, board.TickID, result.TickID)
	}
	if len(board.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(board.Entries))
	}
	entry := board.Entries[0]
	if entry.Path != "状态.身体" || entry.From != "健康" || entry.To != "疲惫" {
		t.Fatalf("entry = %+v, want 状态.身体 健康 -> 疲惫", entry)
	}
	if len(entry.RuleIDs) != 1 || entry.RuleIDs[0] != "r_board" {
		t.Fatalf("rule ids = %v, want [r_board]", entry.RuleIDs)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("board not persisted with the tick: %v", err)
	}
}

// TestRunTickAggregatesOnePathPerTick covers two rules changing one path in the
// same tick: one version bump, earliest from, latest to, deduplicated rules.
func TestRunTickAggregatesOnePathPerTick(t *testing.T) {
	fx, boards, _ := newBoardFixture(t)
	if err := fx.entity.Set("状态.身体", "健康"); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	addStateRule(fx, "r_first", "状态.身体", "疲惫")
	addStateRule(fx, "r_second", "状态.身体", "虚弱")

	result, err := fx.server.RunTick("", TickOptions{})
	if err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if len(result.Transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(result.Transitions))
	}
	board := boards.Board("猫猫")
	if board == nil || board.Version != 1 || len(board.Entries) != 1 {
		t.Fatalf("board = %+v, want version 1 with one entry", board)
	}
	entry := board.Entries[0]
	if entry.From != "健康" || entry.To != "虚弱" {
		t.Fatalf("entry = %+v, want 健康 -> 虚弱", entry)
	}
	if len(entry.RuleIDs) != 2 || entry.RuleIDs[0] != "r_first" || entry.RuleIDs[1] != "r_second" {
		t.Fatalf("rule ids = %v, want [r_first r_second]", entry.RuleIDs)
	}
}

// TestRunTickSurvivesBoardWriteFailure covers failure isolation: a board save
// error is logged, the tick still succeeds and the in-memory board keeps the
// change.
func TestRunTickSurvivesBoardWriteFailure(t *testing.T) {
	fx := newTickFixture(t)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	boards := eventboard.New(filepath.Join(blocker, eventboard.FILE_NAME))
	fx.server.SetEventBoards(boards)
	if err := fx.entity.Set("状态.身体", "健康"); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	addStateRule(fx, "r_failing_board", "状态.身体", "疲惫")

	result, err := fx.server.RunTick("", TickOptions{})
	if err != nil {
		t.Fatalf("tick must survive a board write failure: %v", err)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(result.Transitions))
	}
	if board := boards.Board("猫猫"); board == nil || board.Version != 1 || len(board.Entries) != 1 {
		t.Fatalf("in-memory board = %+v, want the change despite the failed save", board)
	}
}
