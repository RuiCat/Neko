// Multi-subject, cross-entity and emit engine tests.
package engine

import (
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func newMultiStore(t *testing.T) *entity.Store {
	t.Helper()
	store := newTestStore(t)
	store.AddTemplate("场景", map[string]any{"事件": []any{}, "在场": []any{}})
	for _, id := range []string{"客厅", "场景"} {
		if _, err := store.Create(id, "场景", map[string]any{"事件": []any{}, "在场": []any{"猫猫", "梦梦"}}, nil); err != nil {
			t.Fatalf("创建场景 %s 失败: %v", id, err)
		}
	}
	return store
}

func TestMultiSubjectRightSidePath(t *testing.T) {
	store := newMultiStore(t)
	setPath(t, store, "猫猫", "状态.位置", "客厅")
	setPath(t, store, "梦梦", "状态.位置", "客厅")
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_same_place", Match: "NPC[*]", Enabled: true,
		Subjects: []string{"NPC[*]", "NPC[*]"},
		Branches: []Branch{{
			If: stringPtr("$1.状态.位置 = $2.状态.位置"),
			Then: []Op{
				{Op: "set", Subject: 1, Path: "状态.情绪", Value: "开心"},
				{Op: "set", Subject: 2, Path: "状态.情绪", Value: "开心"},
			},
		}},
	})
	result := machine.Tick("")
	if result.Status != "ok" {
		t.Errorf("status = %s, 期望 ok", result.Status)
	}
	expectPath(t, store, "猫猫", "状态.情绪", "开心")
	expectPath(t, store, "梦梦", "状态.情绪", "开心")
}

func TestMultiSubjectCrossEntity(t *testing.T) {
	store := newMultiStore(t)
	setPath(t, store, "猫猫", "身体.体力", 10.0)
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_care", Match: "NPC[*]", Enabled: true,
		Subjects: []string{"NPC[*]", "NPC[*]"},
		Branches: []Branch{{
			If: stringPtr("$1.身体.体力 < 20 ∧ $2.身体.体力 > 50"),
			Then: []Op{
				{Op: "set", Subject: 2, Path: "状态.情绪", Value: "开心"},
				{Op: "set", Subject: 1, Path: "标记.被关心", Value: true},
			},
		}},
	})
	result := machine.Tick("")
	if result.Status != "ok" {
		t.Errorf("status = %s, 期望 ok", result.Status)
	}
	expectPath(t, store, "猫猫", "标记.被关心", true)
	expectPath(t, store, "梦梦", "状态.情绪", "开心")
}

func TestGlobalEntityOpAppend(t *testing.T) {
	store := newMultiStore(t)
	setPath(t, store, "猫猫", "身体.体力", 10.0)
	machine := New(store)
	machine.AddRule(newRule("r_emit", 0,
		Branch{
			If:   stringPtr("{e}.身体.体力 < 20"),
			Then: []Op{{Op: "append", Entity: "客厅", Path: "事件", Value: "猫猫累了"}},
		},
	))
	machine.Tick("")
	events, ok := store.Get("客厅").GetOrNone("事件").([]any)
	if !ok {
		t.Fatalf("客厅.事件 不是列表: %#v", store.Get("客厅").GetOrNone("事件"))
	}
	found := false
	for _, event := range events {
		if event == "猫猫累了" {
			found = true
		}
	}
	if !found {
		t.Errorf("客厅.事件 应包含 猫猫累了, 实际 %+v", events)
	}
}

func TestSameEntityNotDoubleBound(t *testing.T) {
	store := newMultiStore(t)
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_no_self", Match: "NPC[*]", Enabled: true,
		Subjects: []string{"NPC[*]", "NPC[*]"},
		Branches: []Branch{{
			If:   stringPtr("true"),
			Then: []Op{{Op: "set", Subject: 1, Path: "标记.配对", Value: true}},
		}},
	})
	result := machine.Tick("")
	if result.Status != "ok" {
		t.Errorf("status = %s, 期望 ok", result.Status)
	}
}

func TestEmitOp(t *testing.T) {
	store := newMultiStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_emit_default", 0,
		Branch{Then: []Op{{Op: "emit", Value: "猫猫打翻了水杯"}}},
	))
	result := machine.Tick("猫猫")
	if len(result.Transitions) != 1 || len(result.Transitions[0].Operations) != 1 {
		t.Fatalf("emit 应产生一个转移与一个操作: %+v", result.Transitions)
	}
	operation := result.Transitions[0].Operations[0]
	if operation["op"] != "emit" || operation["path"] != "场景.事件" {
		t.Errorf("emit 操作记录错误: %+v", operation)
	}
	events, _ := store.Get("场景").GetOrNone("事件").([]any)
	if len(events) != 1 || events[0] != "猫猫打翻了水杯" {
		t.Errorf("场景.事件 错误: %+v", events)
	}
}
