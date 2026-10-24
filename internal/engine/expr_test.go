// Action-value concat, effect-text and assignment engine tests.
package engine

import (
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func newExprEnv(t *testing.T) (*entity.Store, *StateMachine) {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"身体": map[string]any{"体力": 75.0},
		"位置": map[string]any{"场景": "S001-LR"},
		"状态": map[string]any{"身体": "活跃"},
	})
	for _, id := range []string{"猫猫", "场景"} {
		if _, err := store.Create(id, "NPC", nil, nil); err != nil {
			t.Fatalf("创建实体 %s 失败: %v", id, err)
		}
	}
	return store, New(store)
}

func TestConcatExpressions(t *testing.T) {
	cases := []struct {
		value string
		want  any
	}{
		{`"位置记录:" + $.位置.场景`, "位置记录:S001-LR"},
		{`$.位置.场景 + " 号房间"`, "S001-LR 号房间"},
		{`"前缀:" + $.位置.场景 + "/结束"`, "前缀:S001-LR/结束"},
		{"活跃", "活跃"},
		{`"记录:" + $.不存在的路径`, `"记录:" + $.不存在的路径`},
		{`"位置记录:" + {e}.位置.场景`, "位置记录:S001-LR"},
		{`{e}.位置.场景 + " 号"`, "S001-LR 号"},
	}
	for _, item := range cases {
		store, machine := newExprEnv(t)
		setPath(t, store, "猫猫", "身体.体力", 30.0)
		machine.AddRule(newRule("r_concat", 0,
			Branch{
				If:   stringPtr("{e}.身体.体力 < 50"),
				Then: []Op{{Op: "set", Path: "{e}.状态.触发", Value: item.value}},
			},
		))
		machine.Tick("")
		expectPath(t, store, "猫猫", "状态.触发", item.want)
	}
}

func TestEffectTextIntegration(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"关系": map[string]any{"撒娇欲": 50.0},
		"行为": map[string]any{"傲娇倾向": 50.0},
	})
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatalf("创建实体失败: %v", err)
	}
	setPath(t, store, "猫猫", "行为.主人行为", "抚摸")
	machine := New(store)
	machine.AddRule(newRule("r_effect", 0,
		Branch{
			If:   stringPtr("NPC[*].行为.主人行为 == '抚摸'"),
			Then: []Op{{Op: "set", Path: "{e}.状态.触发", Value: "情境修正:撒娇+15,傲娇-20"}},
		},
	))
	machine.Tick("")
	expectPath(t, store, "猫猫", "关系.撒娇欲", 65.0)
	expectPath(t, store, "猫猫", "行为.傲娇倾向", 30.0)
}

func TestAssignTextInEngine(t *testing.T) {
	cases := []struct {
		value string
		want  any
	}{
		{"安全等级=red", "red"},
		{"安全等级:orange", "orange"},
	}
	for _, item := range cases {
		store := entity.NewStore(t.TempDir())
		store.AddTemplate("NPC", map[string]any{
			"行为": map[string]any{"安全等级": "green"},
			"状态": map[string]any{"身体": "活跃"},
		})
		if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
			t.Fatalf("创建实体失败: %v", err)
		}
		machine := New(store)
		machine.AddRule(newRule("r_test_assign", 0,
			Branch{Then: []Op{{Op: "set", Path: "{e}.状态.触发", Value: item.value}}},
		))
		machine.Tick("")
		expectPath(t, store, "猫猫", "行为.安全等级", item.want)
	}
}
