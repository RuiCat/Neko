// Ported engine assertions from astrbox-plugin/tests/test_engine.py,
// test_engine_expr.py and test_engine_multi.py, plus Go-specific checks for the
// exported API and the spec-mandated branch_race conflict.
package engine

import (
	"encoding/json"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func stringPtr(value string) *string { return &value }

func newTestStore(t *testing.T) *entity.Store {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"状态":   map[string]any{"身体": "活跃", "情绪": "平静"},
		"状态定义": map[string]any{"身体": []any{"活跃", "疲惫", "虚弱", "昏厥"}, "情绪": []any{"平静", "开心", "难过"}},
		"身体":   map[string]any{"体力": 75.0},
	})
	for _, id := range []string{"猫猫", "梦梦"} {
		if _, err := store.Create(id, "NPC", nil, nil); err != nil {
			t.Fatalf("创建实体 %s 失败: %v", id, err)
		}
	}
	return store
}

func setPath(t *testing.T, store *entity.Store, entityID, path string, value any) {
	t.Helper()
	ent := store.Get(entityID)
	if ent == nil {
		t.Fatalf("实体 %s 不存在", entityID)
	}
	if err := ent.Set(path, value); err != nil {
		t.Fatalf("设置 %s.%s 失败: %v", entityID, path, err)
	}
}

func expectPath(t *testing.T, store *entity.Store, entityID, path string, want any) {
	t.Helper()
	ent := store.Get(entityID)
	if ent == nil {
		t.Fatalf("实体 %s 不存在", entityID)
	}
	got := ent.GetOrNone(path)
	if !equalValues(got, want) {
		t.Errorf("%s.%s = %#v, 期望 %#v", entityID, path, got, want)
	}
}

func newRule(id string, priority int, branches ...Branch) Rule {
	return Rule{ID: id, Match: "NPC[*]", Priority: priority, Enabled: true, Branches: branches}
}

func TestBranchShortCircuit(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_fatigue", 0,
		Branch{If: stringPtr("NPC[*].身体.体力 < 10"), State: map[string]any{"身体": "昏厥"}},
		Branch{If: stringPtr("NPC[*].身体.体力 < 20"), State: map[string]any{"身体": "疲惫"}},
		Branch{Else: true},
	))
	setPath(t, store, "猫猫", "身体.体力", 15.0)
	result := machine.Tick("")
	if result.Status != "ok" {
		t.Errorf("status = %s, 期望 ok", result.Status)
	}
	expectPath(t, store, "猫猫", "状态.身体", "疲惫")
}

func TestDeepBranch(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_deep", 0,
		Branch{If: stringPtr("NPC[*].身体.体力 < 10"), State: map[string]any{"身体": "昏厥"}},
		Branch{If: stringPtr("NPC[*].身体.体力 < 20"), State: map[string]any{"身体": "疲惫"}},
		Branch{Else: true},
	))
	setPath(t, store, "猫猫", "身体.体力", 5.0)
	machine.Tick("")
	expectPath(t, store, "猫猫", "状态.身体", "昏厥")
}

func TestElseDefault(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_default", 0,
		Branch{If: stringPtr("NPC[*].身体.体力 < 20"), State: map[string]any{"身体": "疲惫"}},
		Branch{Else: true, State: map[string]any{"身体": "活跃"}},
	))
	setPath(t, store, "猫猫", "身体.体力", 50.0)
	machine.Tick("")
	expectPath(t, store, "猫猫", "状态.身体", "活跃")
}

func TestSideEffects(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_effect", 0,
		Branch{
			If:   stringPtr("NPC[*].身体.体力 < 20"),
			Then: []Op{{Op: "set", Path: "{e}.标记.疲劳", Value: true}},
		},
	))
	setPath(t, store, "猫猫", "身体.体力", 10.0)
	machine.Tick("")
	expectPath(t, store, "猫猫", "标记.疲劳", true)
}

func TestConflictNoMatch(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_conflict", 0,
		Branch{If: stringPtr("NPC[*].身体.体力 < 1")},
	))
	setPath(t, store, "猫猫", "身体.体力", 50.0)
	result := machine.Tick("")
	if result.Status != "conflict" {
		t.Errorf("status = %s, 期望 conflict", result.Status)
	}
	if len(result.Conflicts) == 0 {
		t.Fatal("应至少产生一个冲突")
	}
	if result.Conflicts[0].Type != "no_branch_match" {
		t.Errorf("冲突类型 = %s, 期望 no_branch_match", result.Conflicts[0].Type)
	}
}

func TestResolveConflict(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_conflict2", 0,
		Branch{If: stringPtr("NPC[*].身体.体力 < 1")},
	))
	setPath(t, store, "猫猫", "身体.体力", 50.0)
	result := machine.Tick("")
	var conflict Conflict
	for _, candidate := range result.Conflicts {
		if candidate.EntityID == "猫猫" {
			conflict = candidate
		}
	}
	if conflict.ConflictID == "" {
		t.Fatal("应至少产生一个猫猫的冲突")
	}
	ok := machine.ResolveConflict(conflict.ConflictID, map[string]any{"身体": "疲惫"}, "LLM 认为该休息")
	if !ok {
		t.Error("ResolveConflict 应返回 true")
	}
	expectPath(t, store, "猫猫", "状态.身体", "疲惫")
}

func TestPriority(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_low", 10,
		Branch{If: stringPtr("NPC[*].身体.体力 < 30"), State: map[string]any{"身体": "疲惫"}},
	))
	machine.AddRule(newRule("r_high", 100,
		Branch{If: stringPtr("NPC[*].身体.体力 < 30"), State: map[string]any{"身体": "昏厥"}},
	))
	setPath(t, store, "猫猫", "身体.体力", 20.0)
	machine.Tick("")
	expectPath(t, store, "猫猫", "状态.身体", "昏厥")
}

func TestHighPriorityRulesNotStarved(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	for index := 0; index < 300; index++ {
		machine.AddRule(Rule{
			ID:       "r_low_" + string(rune('A'+index%26)) + string(rune('a'+index/26)),
			Match:    "NPC[*]",
			Priority: 0,
			Enabled:  true,
			Branches: []Branch{{
				Then: []Op{{Op: "set", Path: "{e}.状态.触发", Value: "低优先级触发" + string(rune('A'+index%26)) + string(rune('a'+index/26))}},
			}},
		})
	}
	machine.AddRule(Rule{
		ID:       "r_safety_high",
		Match:    "NPC[*]",
		Priority: 100,
		Enabled:  true,
		Branches: []Branch{{
			Then: []Op{{Op: "set", Path: "{e}.状态.安全等级", Value: "red"}},
		}},
	})
	result := machine.Tick("")
	expectPath(t, store, "猫猫", "状态.安全等级", "red")
	if result.RulesEvaluated < 301 {
		t.Errorf("应评估全部 301 条规则, 实际 %d", result.RulesEvaluated)
	}
}

func TestSameValueSetNoTransition(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_dup", 0,
		Branch{Then: []Op{{Op: "set", Path: "{e}.状态.触发", Value: "相同值"}}},
	))
	first := machine.Tick("")
	if len(first.Transitions) != 2 {
		t.Errorf("首次写入两个实体应有 2 转移, 实际 %d", len(first.Transitions))
	}
	second := machine.Tick("")
	if len(second.Transitions) != 0 {
		t.Errorf("同值写入不应产生转移, 实际 %d", len(second.Transitions))
	}
}

func clampEnv(t *testing.T) (*entity.Store, *StateMachine) {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"探索": map[string]any{"好奇心": 60.0},
		"__annotations__": map[string]any{
			"探索.好奇心": map[string]any{"type": "int", "min": 0.0, "max": 100.0},
		},
	})
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatalf("创建实体失败: %v", err)
	}
	return store, New(store)
}

func TestClampByAnnotationAdd(t *testing.T) {
	store, machine := clampEnv(t)
	cat := store.Get("猫猫")
	machine.applyOp(cat, Op{Op: "add", Path: "探索.好奇心", Value: 200.0}, machine.newEntityResolver(cat), nil)
	if got := entityNumber(t, cat, "探索.好奇心"); got != 100 {
		t.Errorf("add 上限应为 100, 实际 %v", got)
	}
}

func TestClampByAnnotationAddLower(t *testing.T) {
	store, machine := clampEnv(t)
	cat := store.Get("猫猫")
	machine.applyOp(cat, Op{Op: "add", Path: "探索.好奇心", Value: -200.0}, machine.newEntityResolver(cat), nil)
	if got := entityNumber(t, cat, "探索.好奇心"); got != 0 {
		t.Errorf("add 下限应为 0, 实际 %v", got)
	}
}

func TestClampByAnnotationSet(t *testing.T) {
	store, machine := clampEnv(t)
	cat := store.Get("猫猫")
	machine.applyOp(cat, Op{Op: "set", Path: "探索.好奇心", Value: 999.0}, machine.newEntityResolver(cat), nil)
	if got := entityNumber(t, cat, "探索.好奇心"); got != 100 {
		t.Errorf("set 上限应为 100, 实际 %v", got)
	}
}

func TestBranchRaceConflict(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_low", 10,
		Branch{Then: []Op{{Op: "set", Path: "状态.身体", Value: "疲惫"}}},
	))
	machine.AddRule(newRule("r_high", 100,
		Branch{Then: []Op{{Op: "set", Path: "状态.身体", Value: "昏厥"}}},
	))
	result := machine.Tick("猫猫")
	if result.Status != "conflict" {
		t.Errorf("status = %s, 期望 conflict", result.Status)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Type != "branch_race" {
		t.Fatalf("应产生一个 branch_race 冲突, 实际 %+v", result.Conflicts)
	}
	expectPath(t, store, "猫猫", "状态.身体", "昏厥")
	if !machine.ResolveConflict(result.Conflicts[0].ConflictID, map[string]any{"身体": "虚弱"}, "仲裁") {
		t.Error("ResolveConflict 应返回 true")
	}
	expectPath(t, store, "猫猫", "状态.身体", "虚弱")
}

func TestTickResultJSONSnakeCase(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_json", 0,
		Branch{Then: []Op{{Op: "set", Path: "{e}.状态.身体", Value: "疲惫"}}},
	))
	result := machine.Tick("猫猫")
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	for _, key := range []string{"tick_id", "status", "transitions", "conflicts", "hint"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("TickResult JSON 缺少 %s: %s", key, raw)
		}
	}
	if _, ok := payload["reply_hint"]; ok {
		t.Errorf("TickResult 不应输出 reply_hint: %s", raw)
	}
	transitions, ok := payload["transitions"].([]any)
	if !ok || len(transitions) != 1 {
		t.Fatalf("transitions 输出错误: %s", raw)
	}
	transition := transitions[0].(map[string]any)
	for _, key := range []string{"entity_id", "rule_id", "from_state", "to_state", "operations"} {
		if _, ok := transition[key]; !ok {
			t.Errorf("Transition JSON 缺少 %s: %s", key, raw)
		}
	}
}

func TestRuleManagement(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_a", 1, Branch{Then: []Op{{Op: "set", Path: "标记.a", Value: true}}}))
	machine.AddRule(newRule("r_b", 2, Branch{Then: []Op{{Op: "set", Path: "标记.b", Value: true}}}))
	replacement := newRule("r_a", 9, Branch{Then: []Op{{Op: "set", Path: "标记.a", Value: false}}})
	machine.AddRule(replacement)
	if got := len(machine.Rules()); got != 2 {
		t.Errorf("同 id 应覆盖, 规则数 = %d", got)
	}
	if rule := machine.GetRule("r_a"); rule == nil || rule.Priority != 9 {
		t.Errorf("GetRule 未返回覆盖后的规则: %+v", rule)
	}
	if !machine.RemoveRule("r_a") {
		t.Error("RemoveRule 应返回 true")
	}
	if machine.RemoveRule("r_a") {
		t.Error("重复删除应返回 false")
	}
	if rule := machine.GetRule("r_a"); rule != nil {
		t.Errorf("删除后 GetRule 应为 nil: %+v", rule)
	}
}

func TestScopeFilter(t *testing.T) {
	store := newTestStore(t)
	machine := New(store)
	machine.AddRule(newRule("r_scope", 0,
		Branch{Then: []Op{{Op: "set", Path: "{e}.标记.作用域", Value: true}}},
	))
	machine.Tick("猫猫")
	expectPath(t, store, "猫猫", "标记.作用域", true)
	if got := store.Get("梦梦").GetOrNone("标记.作用域"); got != nil {
		t.Errorf("scope 应只作用于猫猫, 梦梦 = %#v", got)
	}
}
