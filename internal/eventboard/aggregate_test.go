// aggregate_test.go covers the tick-transition -> per-entity change folding.
package eventboard

import (
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
