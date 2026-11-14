// sceneEventProjection_test.go covers the bounded projection of a scene's
// unbounded event stream onto `事件.最新` (board apply, aggregation, registry).
package eventboard

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
)

// sceneWrite builds one event-stream write as the engine reports it.
func sceneWrite(from, to any) engine.EntityWrite {
	return engine.EntityWrite{EntityID: "S001-LR", RuleID: "r_event", Path: "事件", From: from, To: to}
}

func TestProjectSceneEventChange(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	change, ok := projectSceneEventChange(Change{
		Path: "事件", From: []any{"敲门"}, To: []any{"敲门", "开灯"}, FromKnown: true, RuleIDs: []string{"r1"},
	})
	if !ok {
		t.Fatal("expected a projected change")
	}
	if change.Path != "事件.最新" || change.From != "敲门" || change.To != "开灯" || !change.FromKnown {
		t.Fatalf("projected = %+v, want 事件.最新 敲门 -> 开灯", change)
	}
	if len(change.RuleIDs) != 1 || change.RuleIDs[0] != "r1" {
		t.Fatalf("rule ids = %v, want [r1]", change.RuleIDs)
	}
}

func TestProjectSceneEventFirstEntry(t *testing.T) {
	change, ok := projectSceneEventChange(Change{Path: "事件", To: []any{"开灯"}, FromKnown: true})
	if !ok {
		t.Fatal("expected a projected change")
	}
	if change.From != nil || change.FromKnown {
		t.Fatalf("first entry should have no from: %+v", change)
	}
	if change.To != "开灯" {
		t.Fatalf("to = %#v, want 开灯", change.To)
	}
}

func TestProjectSceneEventPreviewsStructuredValue(t *testing.T) {
	change, ok := projectSceneEventChange(Change{
		Path: "事件", To: []any{map[string]any{"内容": "打雷", "强度": 3}}, FromKnown: true,
	})
	if !ok {
		t.Fatal("expected a projected change")
	}
	text, _ := change.To.(string)
	if !strings.Contains(text, "打雷") || !strings.Contains(text, "强度") {
		t.Fatalf("structured preview = %q, want the JSON text", text)
	}
}

func TestProjectSceneEventTruncatesPreview(t *testing.T) {
	long := strings.Repeat("很", sceneEventPreviewMaxRunes+80)
	change, ok := projectSceneEventChange(Change{Path: "事件", To: []any{long}, FromKnown: true})
	if !ok {
		t.Fatal("expected a projected change")
	}
	text, _ := change.To.(string)
	if runes := []rune(text); len(runes) != sceneEventPreviewMaxRunes+1 || !strings.HasSuffix(text, "…") {
		t.Fatalf("preview length = %d, want %d + ellipsis", len(runes), sceneEventPreviewMaxRunes)
	}
}

func TestProjectSceneEventDropsEmptyLists(t *testing.T) {
	for _, change := range []Change{
		{Path: "事件", From: []any{}, To: []any{}},
		{Path: "事件", From: nil, To: nil},
	} {
		if _, ok := projectSceneEventChange(change); ok {
			t.Fatalf("empty lists should drop: %+v", change)
		}
	}
}

func TestAggregateProjectsSceneEvents(t *testing.T) {
	groups := aggregateWrites([]engine.EntityWrite{
		sceneWrite([]any{"敲门"}, []any{"敲门", "开灯"}),
		sceneWrite([]any{"敲门", "开灯"}, []any{"敲门", "开灯", "电视关了"}),
		{EntityID: "S001-LR", Path: "氛围.光照", From: "亮", To: "暗"},
	})
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want one entity", groups)
	}
	changes := groups[0].Changes
	if len(changes) != 2 {
		t.Fatalf("changes = %+v, want two paths", changes)
	}
	event := changes[0]
	if event.Path != "事件" || !valuesEqual(event.To, []any{"敲门", "开灯", "电视关了"}) {
		t.Fatalf("folded event change = %+v, want the latest list", event)
	}
	if changes[1].Path != "氛围.光照" {
		t.Fatalf("second change = %+v, want the ordinary path", changes[1])
	}
}

func TestBoardAppliesProjectedEntry(t *testing.T) {
	board := newBoard("S001-LR")
	at := time.Date(2026, 11, 10, 18, 0, 0, 0, time.Local)
	if !board.apply([]Change{{Path: "事件", From: []any{"敲门"}, To: []any{"敲门", "开灯"}, FromKnown: true}}, "tick-1", at) {
		t.Fatal("expected the board to change")
	}
	var projected *Entry
	for index := range board.Entries {
		if board.Entries[index].Path == "事件" {
			t.Fatalf("board stored the raw stream: %+v", board.Entries[index])
		}
		if board.Entries[index].Path == "事件.最新" {
			projected = &board.Entries[index]
		}
	}
	if projected == nil || projected.From != "敲门" || projected.To != "开灯" {
		t.Fatalf("projected entry = %+v, want 敲门 -> 开灯", projected)
	}
	if board.Version != 1 {
		t.Fatalf("version = %d, want 1", board.Version)
	}
	if !board.apply([]Change{{Path: "事件", From: []any{"敲门", "开灯"}, To: []any{"敲门", "开灯", "电视关了"}, FromKnown: true}}, "tick-2", at.Add(time.Minute)) {
		t.Fatal("expected the second event to update the projected entry")
	}
	projected = nil
	for index := range board.Entries {
		if board.Entries[index].Path == "事件.最新" {
			projected = &board.Entries[index]
		}
	}
	if projected == nil || projected.From != "开灯" || projected.To != "电视关了" {
		t.Fatalf("second projection = %+v, want 开灯 -> 电视关了", projected)
	}
	if len(board.Entries) != 1 {
		t.Fatalf("entries = %+v, want one projected entry", board.Entries)
	}
}

func TestBoardDoesNotVersionUnchangedSceneEvent(t *testing.T) {
	board := newBoard("S001-LR")
	at := time.Date(2026, 11, 10, 18, 0, 0, 0, time.Local)
	if !board.apply([]Change{{Path: "事件", From: []any{"a"}, To: []any{"a", "b"}, FromKnown: true}}, "tick-1", at) {
		t.Fatal("expected the board to change")
	}
	same := Change{Path: "事件", From: []any{"a", "b"}, To: []any{"a", "b"}, FromKnown: true}
	if board.apply([]Change{same}, "tick-2", at.Add(time.Minute)) {
		t.Fatal("an unchanged event list must not bump the version")
	}
}

func TestRegistryRecordProjectsSceneEvents(t *testing.T) {
	registry := New(filepath.Join(t.TempDir(), "blackboard.json"))
	at := time.Date(2026, 11, 10, 18, 0, 0, 0, time.Local)
	change := Change{Path: "事件", From: []any{"a"}, To: []any{"a", "b"}, FromKnown: true}
	if err := registry.Record("S001-LR", []Change{change}, "direct:mcp", at); err != nil {
		t.Fatalf("record: %v", err)
	}
	board := registry.Board("S001-LR")
	if board == nil || len(board.Entries) != 1 {
		t.Fatalf("board = %+v, want one entry", board)
	}
	entry := board.Entries[0]
	if entry.Path != "事件.最新" || entry.From != "a" || entry.To != "b" {
		t.Fatalf("entry = %+v, want 事件.最新 a -> b", entry)
	}
}

// TestEndToEndSceneTickReachesBoard runs a real tick that appends a scene event
// and checks the board receives only the bounded projection.
func TestEndToEndSceneTickReachesBoard(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("场景", map[string]any{"事件": []any{}})
	if _, err := store.Create("S001-LR", "场景", map[string]any{"事件": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "NPC", map[string]any{
		"位置": map[string]any{"场景": "S001-LR"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	machine := engine.New(store)
	machine.AddRule(engine.Rule{
		ID: "r_scene", Match: "猫猫", Enabled: true,
		Branches: []engine.Branch{{Else: true, Then: []engine.Op{{
			Op: "append", Entity: "场景", Path: "事件", Value: "进入客厅",
		}}}},
	})
	result := machine.Tick("猫猫")
	if result.Status != "ok" {
		t.Fatalf("tick status = %s", result.Status)
	}
	registry := New(filepath.Join(t.TempDir(), "blackboard.json"))
	if err := registry.Apply(result); err != nil {
		t.Fatalf("apply: %v", err)
	}
	board := registry.Board("S001-LR")
	if board == nil {
		t.Fatalf("board = nil, want the scene board")
	}
	// 场景板同时会有在场的自动同步（同一 tick），这里只钉事件投影那一条。
	var projected *Entry
	for index := range board.Entries {
		if board.Entries[index].Path == "事件" {
			t.Fatalf("board stored the raw stream: %+v", board.Entries[index])
		}
		if board.Entries[index].Path == "事件.最新" {
			projected = &board.Entries[index]
		}
	}
	if projected == nil || projected.To != "进入客厅" {
		t.Fatalf("entry = %+v, want 事件.最新 -> 进入客厅", projected)
	}
}
