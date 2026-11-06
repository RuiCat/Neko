// registry_test.go covers the shared registry: baseline observation, tick
// application, persistence, load failures and reader copies.
package eventboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
)

// nestedFrom builds the engine's nested previous-state snapshot for one dotted
// path ({"状态":{"身体":"健康"}} for "状态.身体").
func nestedFrom(path string, value any) map[string]any {
	segments := strings.Split(path, ".")
	root := map[string]any{}
	current := root
	for index, segment := range segments {
		if index == len(segments)-1 {
			current[segment] = value
			return root
		}
		next := map[string]any{}
		current[segment] = next
		current = next
	}
	return root
}

// setTick builds one TickResult with a single `set` transition.
func setTick(tickID, entity, path string, from, to any) engine.TickResult {
	return engine.TickResult{TickID: tickID, Transitions: []engine.Transition{{
		EntityID: entity, RuleID: "r1",
		FromState:  nestedFrom(path, from),
		Operations: []map[string]any{setOp(path, to)},
	}}}
}

// TestRegistryObserveCreatesBaseline covers first observation: a board exists
// with no entries, no version and no timestamp, and nothing is persisted yet.
func TestRegistryObserveCreatesBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	registry := New(path)
	registry.Observe([]string{"梦梦", ""})

	board := registry.Board("梦梦")
	if board == nil {
		t.Fatal("baseline board missing")
	}
	if board.Version != 0 || len(board.Entries) != 0 || board.UpdatedAt != "" || board.TickID != "" {
		t.Fatalf("baseline must be empty: %+v", board)
	}
	if registry.Board("不存在") != nil {
		t.Fatal("unknown entity must have no board")
	}
	if len(registry.Boards()) != 1 {
		t.Fatalf("boards = %d, want 1", len(registry.Boards()))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("baseline observation wrote the board file: %v", err)
	}
}

// TestRegistryApplyBumpsVersionOncePerTick covers a new entity board created by
// its first change: entries appear and the version counts ticks, not paths.
func TestRegistryApplyBumpsVersionOncePerTick(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), FILE_NAME))
	result := engine.TickResult{TickID: "tick-1", Transitions: []engine.Transition{
		{EntityID: "梦梦", RuleID: "r1", FromState: nestedFrom("心情.当前", "满足"), Operations: []map[string]any{setOp("心情.当前", "开心")}},
		{EntityID: "梦梦", RuleID: "r2", FromState: nestedFrom("身体.体力", 90), Operations: []map[string]any{setOp("身体.体力", 92)}},
	}}
	if err := registry.Apply(result); err != nil {
		t.Fatalf("apply: %v", err)
	}
	board := registry.Board("梦梦")
	if board == nil || board.Version != 1 || len(board.Entries) != 2 {
		t.Fatalf("board = %+v, want version 1 with 2 entries", board)
	}
	if board.TickID != "tick-1" {
		t.Fatalf("tick_id = %q, want tick-1", board.TickID)
	}
	if _, err := time.Parse(time.RFC3339, board.UpdatedAt); err != nil {
		t.Fatalf("updated_at %q is not RFC3339: %v", board.UpdatedAt, err)
	}
	if board.Entries[0].From != "满足" || board.Entries[0].To != "开心" {
		t.Fatalf("entry = %+v, want 满足 -> 开心", board.Entries[0])
	}

	if err := registry.Apply(setTick("tick-2", "梦梦", "心情.当前", "开心", "兴奋")); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	board = registry.Board("梦梦")
	if board.Version != 2 || len(board.Entries) != 2 {
		t.Fatalf("board = %+v, want version 2 with 2 entries (in-place update)", board)
	}
	if board.Entries[0].From != "开心" || board.Entries[0].To != "兴奋" {
		t.Fatalf("updated entry = %+v, want 开心 -> 兴奋", board.Entries[0])
	}
}

// TestRegistryApplyPersistsAndReloads covers the tick-synced save and startup
// load: a reloaded registry answers the same board.
func TestRegistryApplyPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	registry := New(path)
	registry.Observe([]string{"猫猫"})
	if err := registry.Apply(setTick("tick-1", "猫猫", "状态.身体", "健康", "疲惫")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read board file: %v", err)
	}
	var file boardFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("board file is not valid JSON: %v", err)
	}
	if len(file.Boards) != 1 || file.Boards[0].Entity != "猫猫" {
		t.Fatalf("persisted boards = %+v, want one 猫猫 board", file.Boards)
	}

	reloaded := New(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	board := reloaded.Board("猫猫")
	if board == nil || board.Version != 1 || board.TickID != "tick-1" || len(board.Entries) != 1 {
		t.Fatalf("reloaded board = %+v", board)
	}
	if board.Entries[0].Path != "状态.身体" || board.Entries[0].From != "健康" || board.Entries[0].To != "疲惫" {
		t.Fatalf("reloaded entry = %+v", board.Entries[0])
	}
}

// TestRegistryLoadMissingFileIsEmpty covers the first start: no board file is
// an empty board set, not an error.
func TestRegistryLoadMissingFileIsEmpty(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), FILE_NAME))
	if err := registry.Load(); err != nil {
		t.Fatalf("load missing file: %v", err)
	}
	if len(registry.Boards()) != 0 {
		t.Fatalf("boards = %d, want 0", len(registry.Boards()))
	}
}

// TestRegistryLoadRejectsCorruptFile covers abnormal data: the error is
// returned so the caller can log it and continue with empty boards.
func TestRegistryLoadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	registry := New(path)
	if err := registry.Load(); err == nil {
		t.Fatal("corrupt board file must report an error")
	}
	if len(registry.Boards()) != 0 {
		t.Fatalf("corrupt load left boards behind: %d", len(registry.Boards()))
	}
}

// TestRegistryApplyReportsSaveFailure covers a failed save while the tick keeps
// going: Apply reports the error and the in-memory board still holds the change.
func TestRegistryApplyReportsSaveFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	registry := New(filepath.Join(blocker, FILE_NAME))
	if err := registry.Apply(setTick("tick-1", "猫猫", "状态.身体", "健康", "疲惫")); err == nil {
		t.Fatal("apply must report the save failure")
	}
	if board := registry.Board("猫猫"); board == nil || board.Version != 1 {
		t.Fatalf("in-memory board = %+v, want version 1 despite save failure", board)
	}
}

// TestRegistryApplyWithoutChangesDoesNotSave covers an unchanged tick: no file
// is written and no error is returned.
func TestRegistryApplyWithoutChangesDoesNotSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	registry := New(path)
	if err := registry.Apply(engine.TickResult{TickID: "tick-1"}); err != nil {
		t.Fatalf("apply unchanged tick: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unchanged tick wrote the board file: %v", err)
	}
}

// TestRegistryBoardsSortedByEntity covers the list order the control plane
// depends on: entity ascending.
func TestRegistryBoardsSortedByEntity(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), FILE_NAME))
	registry.Observe([]string{"beta", "梦梦", "alpha"})
	boards := registry.Boards()
	if len(boards) != 3 {
		t.Fatalf("boards = %d, want 3", len(boards))
	}
	want := []string{"alpha", "beta", "梦梦"}
	for index, entity := range want {
		if boards[index].Entity != entity {
			t.Fatalf("boards[%d] = %s, want %s", index, boards[index].Entity, entity)
		}
	}
}

// TestRegistryBoardReturnsCopy covers read isolation: mutating a returned board
// never reaches the registry.
func TestRegistryBoardReturnsCopy(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), FILE_NAME))
	if err := registry.Apply(setTick("tick-1", "猫猫", "状态.身体", "健康", "疲惫")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	board := registry.Board("猫猫")
	board.Entries[0].To = "被改"
	board.Version = 99
	if got := registry.Board("猫猫"); got.Version != 1 || got.Entries[0].To != "疲惫" {
		t.Fatalf("live board changed through a reader copy: %+v", got)
	}
}

// TestRegistryObserveKeepsChangedBoard covers a second observation after a
// change: the existing entries and version survive.
func TestRegistryObserveKeepsChangedBoard(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), FILE_NAME))
	if err := registry.Apply(setTick("tick-1", "猫猫", "状态.身体", "健康", "疲惫")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	registry.Observe([]string{"猫猫"})
	board := registry.Board("猫猫")
	if board == nil || board.Version != 1 || len(board.Entries) != 1 {
		t.Fatalf("observe reset a changed board: %+v", board)
	}
}
