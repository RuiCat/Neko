// aggregate_test.go covers the tick-transition -> per-entity change folding.
package eventboard

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
)

// setOp builds one `set` operation exactly as the engine emits it.
func setOp(path string, value any) map[string]any {
	return map[string]any{"op": "set", "path": path, "value": value}
}

// stateGroup builds the engine's nested previous-state snapshot
// ({"状态": {...}}), which is what FromState actually contains.
func stateGroup(values map[string]any) map[string]any {
	return map[string]any{"状态": values}
}

// TestAggregateKeepsEarliestFromAndLatestTo covers the same-path rule: one
// change survives with the first from, the last to and deduplicated rule ids.
func TestAggregateKeepsEarliestFromAndLatestTo(t *testing.T) {
	groups := aggregateTransitions([]engine.Transition{
		{
			EntityID: "梦梦", RuleID: "r1",
			FromState:  stateGroup(map[string]any{"身体": "疲惫", "心情": "满足"}),
			Operations: []map[string]any{setOp("状态.身体", "健康"), setOp("状态.心情", "开心")},
		},
		{
			EntityID: "梦梦", RuleID: "r2",
			FromState:  stateGroup(map[string]any{"身体": "健康", "心情": "开心"}),
			Operations: []map[string]any{setOp("状态.身体", "虚弱"), setOp("状态.心情", "平静")},
		},
		{
			EntityID: "猫猫", RuleID: "r1",
			FromState:  stateGroup(map[string]any{"身体": "健康"}),
			Operations: []map[string]any{setOp("状态.身体", "疲惫")},
		},
	})
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Entity != "梦梦" || groups[1].Entity != "猫猫" {
		t.Fatalf("entity order = %s,%s", groups[0].Entity, groups[1].Entity)
	}
	changes := groups[0].Changes
	if len(changes) != 2 {
		t.Fatalf("梦梦 changes = %d, want 2 (one per path)", len(changes))
	}
	body := changes[0]
	if body.Path != "状态.身体" || body.From != "疲惫" || body.To != "虚弱" {
		t.Fatalf("状态.身体 = %+v, want 疲惫 -> 虚弱", body)
	}
	if len(body.RuleIDs) != 2 || body.RuleIDs[0] != "r1" || body.RuleIDs[1] != "r2" {
		t.Fatalf("rule ids = %v, want [r1 r2]", body.RuleIDs)
	}
	mood := changes[1]
	if mood.Path != "状态.心情" || mood.From != "满足" || mood.To != "平静" {
		t.Fatalf("状态.心情 = %+v, want 满足 -> 平静", mood)
	}
}

// TestAggregateDeduplicatesRuleIDs covers one rule touching the same path twice
// (for example two branches writing one field): the id appears once.
func TestAggregateDeduplicatesRuleIDs(t *testing.T) {
	groups := aggregateTransitions([]engine.Transition{{
		EntityID: "猫猫", RuleID: "r_same",
		FromState:  stateGroup(map[string]any{"身体": "健康"}),
		Operations: []map[string]any{setOp("状态.身体", "疲惫"), setOp("状态.身体", "虚弱")},
	}})
	if len(groups) != 1 || len(groups[0].Changes) != 1 {
		t.Fatalf("groups = %+v, want one entity with one change", groups)
	}
	change := groups[0].Changes[0]
	if change.From != "健康" || change.To != "虚弱" {
		t.Fatalf("change = %+v, want 健康 -> 虚弱", change)
	}
	if len(change.RuleIDs) != 1 || change.RuleIDs[0] != "r_same" {
		t.Fatalf("rule ids = %v, want [r_same]", change.RuleIDs)
	}
}

// TestAggregateIgnoresNonSetOperations covers add/append/emit: they are not
// state path replacements and must not produce from -> to entries.
func TestAggregateIgnoresNonSetOperations(t *testing.T) {
	groups := aggregateTransitions([]engine.Transition{{
		EntityID: "猫猫", RuleID: "r1",
		Operations: []map[string]any{
			{"op": "add", "path": "身体.体力", "value": 2},
			{"op": "append", "path": "场景.事件", "value": "敲门"},
			{"op": "emit", "path": "场景.事件", "value": "敲门"},
			{"op": "set", "path": "", "value": "x"},
		},
	}})
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
}

// TestAggregateResolvesNestedAndFlatFromState covers both previous-state
// layouts: the engine's group-content snapshot (no group key) resolves by
// dropping leading segments, a flat dotted key hits directly, and a path absent
// from the snapshot stays nil (null in K1).
func TestAggregateResolvesNestedAndFlatFromState(t *testing.T) {
	nested := aggregateTransitions([]engine.Transition{{
		EntityID: "梦梦", RuleID: "r1",
		FromState:  map[string]any{"心情": map[string]any{"当前": "满足"}},
		Operations: []map[string]any{setOp("状态.心情.当前", "开心")},
	}})
	if got := nested[0].Changes[0].From; got != "满足" {
		t.Fatalf("nested from = %v, want 满足", got)
	}
	flat := aggregateTransitions([]engine.Transition{{
		EntityID: "梦梦", RuleID: "r1",
		FromState:  map[string]any{"状态.心情.当前": "满足"},
		Operations: []map[string]any{setOp("状态.心情.当前", "开心")},
	}})
	if got := flat[0].Changes[0].From; got != "满足" {
		t.Fatalf("flat from = %v, want 满足", got)
	}
	missing := aggregateTransitions([]engine.Transition{{
		EntityID: "梦梦", RuleID: "r1",
		FromState:  map[string]any{"身体": "健康"},
		Operations: []map[string]any{setOp("状态.心情", "开心")},
	}})
	if got := missing[0].Changes[0].From; got != nil {
		t.Fatalf("missing from = %v, want nil", got)
	}
}

// TestAggregateSkipsEmptyEntity covers transitions without an entity id.
func TestAggregateSkipsEmptyEntity(t *testing.T) {
	groups := aggregateTransitions([]engine.Transition{{
		EntityID: "", RuleID: "r1", Operations: []map[string]any{setOp("状态.身体", "疲惫")},
	}})
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
}

// TestAggregateFromKnown covers the FromKnown flag: state-group paths resolve
// from FromState, paths outside it stay unknown so the board can fall back.
func TestAggregateFromKnown(t *testing.T) {
	groups := aggregateTransitions([]engine.Transition{{
		EntityID:   "猫猫",
		RuleID:     "r1",
		FromState:  stateGroup(map[string]any{"情绪": "平静"}),
		Operations: []map[string]any{setOp("状态.情绪", "开心"), setOp("睡眠.疲劳", 20)},
	}})
	if len(groups) != 1 || len(groups[0].Changes) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if !groups[0].Changes[0].FromKnown {
		t.Fatalf("状态组路径应可解析 from: %+v", groups[0].Changes[0])
	}
	if groups[0].Changes[1].FromKnown {
		t.Fatalf("状态组外路径的 from 不应标 known: %+v", groups[0].Changes[1])
	}
}

// TestAggregateWrites covers tick-observed writes: rule ops, text effects and
// implicit @函数 writes become changes; no-net-change paths are skipped. The
// scene event stream is kept here and bounded later by the board projection.
func TestAggregateWrites(t *testing.T) {
	groups := aggregateWrites([]engine.EntityWrite{
		{EntityID: "猫猫", RuleID: "r1", Path: "状态.情绪", From: "平静", To: "开心"},
		{EntityID: "猫猫", RuleID: "r2", Path: "状态.情绪", From: "开心", To: "兴奋"},
		{EntityID: "猫猫", RuleID: "r1", Path: "身体.饱食", From: 40, To: 80},
		{EntityID: "猫猫", RuleID: "r3", Path: "标记.饥饿", From: true, To: true},
		{EntityID: "猫猫", RuleID: "r9", Path: "事件", From: []any{}, To: []any{"x"}},
		{EntityID: "", RuleID: "r1", Path: "状态.情绪", From: "a", To: "b"},
	})
	if len(groups) != 1 || groups[0].Entity != "猫猫" {
		t.Fatalf("groups = %+v", groups)
	}
	changes := groups[0].Changes
	if len(changes) != 3 {
		t.Fatalf("changes = %d, want 3: %+v", len(changes), changes)
	}
	mood := changes[0]
	if mood.Path != "状态.情绪" || mood.From != "平静" || mood.To != "兴奋" {
		t.Fatalf("状态.情绪 = %+v", mood)
	}
	if len(mood.RuleIDs) != 2 || mood.RuleIDs[0] != "r1" || mood.RuleIDs[1] != "r2" {
		t.Fatalf("rule ids = %v", mood.RuleIDs)
	}
	if !mood.FromKnown {
		t.Fatal("observed writes always know the previous value")
	}
	food := changes[1]
	if food.Path != "身体.饱食" || food.From != 40 || food.To != 80 {
		t.Fatalf("身体.饱食 = %+v", food)
	}
	event := changes[2]
	if event.Path != "事件" || !valuesEqual(event.To, []any{"x"}) {
		t.Fatalf("事件 = %+v, want the raw list for the board projection", event)
	}
}

// TestRegistryApplyUsesObservedWrites proves Apply prefers Writes over
// transitions, so effect/function writes reach the board.
func TestRegistryApplyUsesObservedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	registry := New(path)
	result := engine.TickResult{
		TickID: "tick-writes",
		Writes: []engine.EntityWrite{
			{EntityID: "猫猫", RuleID: "r_feed", Path: "身体.饱食", From: 40, To: 80},
		},
	}
	if err := registry.Apply(result); err != nil {
		t.Fatalf("apply: %v", err)
	}
	board := registry.Board("猫猫")
	if board == nil || board.Version != 1 || len(board.Entries) != 1 {
		t.Fatalf("board = %+v", board)
	}
	entry := board.Entries[0]
	if entry.Path != "身体.饱食" || entry.From != 40 || entry.To != 80 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Version != 1 || board.TickID != "tick-writes" {
		t.Fatalf("entry version/tick = %d/%q", entry.Version, board.TickID)
	}
}
