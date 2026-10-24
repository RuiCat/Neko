// Integration tests for the @函数 bridge: rule conditions and action values call
// the real 时段判断 function from scripts/cat_daily.neko.
package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
	nekoFunction "github.com/ruicat/astrbox/internal/function"
	"github.com/ruicat/astrbox/internal/neko"
)

// loadScriptFunction parses one @函数 out of a corpus script.
func loadScriptFunction(t *testing.T, scriptName, functionName string) neko.FuncDef {
	t.Helper()
	category := "behavior"
	if strings.HasPrefix(scriptName, "cat_core") {
		category = "core"
	}
	if strings.HasPrefix(scriptName, "scene_") {
		category = "scene"
	}
	path := filepath.Join("..", "..", "scripts", category, scriptName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取语料 %s 失败: %v", scriptName, err)
	}
	ast := neko.Parse(string(raw))
	for _, definition := range ast.FuncDefs {
		if definition.Name == functionName {
			return definition
		}
	}
	t.Fatalf("%s 中缺少 @函数 %s", scriptName, functionName)
	return neko.FuncDef{}
}

// newFuncCallFixture builds a 世界/猫猫 store plus a machine wired (or not) to
// the real function executor.
func newFuncCallFixture(t *testing.T, withCaller bool) (*StateMachine, *entity.Store) {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("世界", map[string]any{
		"环境": map[string]any{"当前时间": "2026-08-05T15:00:00+08:00"},
	})
	store.AddTemplate("NPC", map[string]any{
		"状态": map[string]any{"触发": "", "时段": ""},
	})
	if _, err := store.Create("世界", "世界", nil, nil); err != nil {
		t.Fatalf("创建 世界 失败: %v", err)
	}
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatalf("创建 猫猫 失败: %v", err)
	}
	executor := nekoFunction.New(store)
	if err := executor.Register(loadScriptFunction(t, "cat_daily.neko", "时段判断")); err != nil {
		t.Fatalf("注册 时段判断 失败: %v", err)
	}
	machine := New(store)
	if withCaller {
		machine.SetFuncCaller(executor)
	}
	return machine, store
}

// afternoonWhen exercises a condition that needs the function executor.
const afternoonWhen = `时段判断(世界.环境.当前时间) == "下午"`

// TestFuncCallerInCondition checks the condition path with and without a caller.
func TestFuncCallerInCondition(t *testing.T) {
	t.Run("设置 caller 时命中", func(t *testing.T) {
		machine, store := newFuncCallFixture(t, true)
		machine.AddRule(Rule{
			ID:      "r_func_when",
			Match:   "NPC[*]",
			When:    stringPtr(afternoonWhen),
			Enabled: true,
			Branches: []Branch{
				{Then: []Op{{Op: "set", Path: "状态.触发", Value: "午后犯困"}}},
			},
		})
		result := machine.Tick("NPC[*]")
		if len(result.Transitions) != 1 {
			t.Fatalf("应命中 1 条规则, 实际 %d (errors=%v)", len(result.Transitions), result.Errors)
		}
		got := store.Get("猫猫").GetOrNone("状态.触发")
		if got != "午后犯困" {
			t.Fatalf("猫猫.状态.触发 = %#v, 期望 午后犯困", got)
		}
	})

	t.Run("多主体规则的条件也能调用函数", func(t *testing.T) {
		machine, store := newFuncCallFixture(t, true)
		machine.AddRule(Rule{
			ID:       "r_func_multi",
			Match:    "NPC[*]",
			Subjects: []string{"NPC[猫猫]", "世界"},
			When:     stringPtr(afternoonWhen),
			Enabled:  true,
			Branches: []Branch{
				{Then: []Op{{Op: "set", Path: "状态.触发", Value: "下午茶"}}},
			},
		})
		result := machine.Tick("NPC[*]")
		if len(result.Transitions) != 1 {
			t.Fatalf("多主体规则应命中 1 次, 实际 %d (errors=%v)", len(result.Transitions), result.Errors)
		}
		got := store.Get("猫猫").GetOrNone("状态.触发")
		if got != "下午茶" {
			t.Fatalf("猫猫.状态.触发 = %#v, 期望 下午茶", got)
		}
	})

	t.Run("未设置 caller 时条件为 false 且不 panic", func(t *testing.T) {
		machine, store := newFuncCallFixture(t, false)
		machine.AddRule(Rule{
			ID:      "r_func_when",
			Match:   "NPC[*]",
			When:    stringPtr(afternoonWhen),
			Enabled: true,
			Branches: []Branch{
				{Then: []Op{{Op: "set", Path: "状态.触发", Value: "午后犯困"}}},
			},
		})
		result := machine.Tick("NPC[*]")
		if len(result.Transitions) != 0 {
			t.Fatalf("未设置 caller 不应命中, 实际 %+v", result.Transitions)
		}
		if result.Status != "ok" {
			t.Fatalf("status = %s, 期望 ok (errors=%v)", result.Status, result.Errors)
		}
		got := store.Get("猫猫").GetOrNone("状态.触发")
		if got != "" {
			t.Fatalf("猫猫.状态.触发 = %#v, 期望空", got)
		}
	})
}

// TestFuncCallerInActionValue checks ops.go function-call value resolution.
func TestFuncCallerInActionValue(t *testing.T) {
	machine, store := newFuncCallFixture(t, true)
	machine.AddRule(Rule{
		ID:      "r_func_action",
		Match:   "NPC[*]",
		Enabled: true,
		Branches: []Branch{
			{Then: []Op{{Op: "set", Path: "状态.时段", Value: `时段判断("2026-08-05T15:00:00+08:00")`}}},
		},
	})
	result := machine.Tick("NPC[*]")
	if len(result.Transitions) != 1 {
		t.Fatalf("动作值函数调用应产生 1 条转移, 实际 %d (errors=%v)", len(result.Transitions), result.Errors)
	}
	got := store.Get("猫猫").GetOrNone("状态.时段")
	if got != "下午" {
		t.Fatalf("猫猫.状态.时段 = %#v, 期望 下午", got)
	}
}

// TestFuncCallerWithCurrentEntityArg covers compiler-rewritten "{e}." arguments
// such as 危险度($[NPC].身体.体力) -> 危险度({e}.身体.体力).
func TestFuncCallerWithCurrentEntityArg(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"身体": map[string]any{"体力": 10},
		"状态": map[string]any{"触发": ""},
	})
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatalf("创建 猫猫 失败: %v", err)
	}
	executor := nekoFunction.New(store)
	if err := executor.Register(loadScriptFunction(t, "cat_core_v2.neko", "危险度")); err != nil {
		t.Fatalf("注册 危险度 失败: %v", err)
	}
	machine := New(store)
	machine.SetFuncCaller(executor)
	machine.AddRule(Rule{
		ID: "r_func_current", Match: "NPC[*]", Enabled: true,
		When:     stringPtr(`危险度({e}.身体.体力) > 50`),
		Branches: []Branch{{Then: []Op{{Op: "set", Path: "状态.触发", Value: "危险"}}}},
	})
	result := machine.Tick("NPC[*]")
	if len(result.Transitions) != 1 {
		t.Fatalf("应命中 1 条规则, 实际 %d (errors=%v)", len(result.Transitions), result.Errors)
	}
}
