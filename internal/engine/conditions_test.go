// Ported condition-evaluator assertions from astrbox-plugin/tests/
// test_engine.py::TestConditions and test_engine_multi.py (condition cases).
package engine

import (
	"fmt"
	"strings"
	"testing"
)

func mapResolver(data map[string]any) ValueResolver {
	return func(path string) (any, error) {
		var current any = data
		for _, key := range strings.Split(path, ".") {
			object, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("路径不存在: %s", path)
			}
			current, ok = object[key]
			if !ok {
				return nil, fmt.Errorf("路径不存在: %s", path)
			}
		}
		return current, nil
	}
}

func checkCondition(t *testing.T, resolver ValueResolver, expr string, want bool) {
	t.Helper()
	got, err := EvaluateCondition(expr, resolver)
	if err != nil {
		t.Errorf("求值 %q 返回错误: %v", expr, err)
		return
	}
	if got != want {
		t.Errorf("求值 %q = %v, 期望 %v", expr, got, want)
	}
}

func TestConditionNumericCompare(t *testing.T) {
	resolver := mapResolver(map[string]any{"体力": 15})
	cases := []struct {
		expr string
		want bool
	}{
		{"体力 < 20", true},
		{"体力 > 20", false},
		{"体力 >= 15", true},
		{"体力 <= 15", true},
		{"体力 == 15", true},
		{"体力 != 20", true},
	}
	for _, item := range cases {
		checkCondition(t, resolver, item.expr, item.want)
	}
}

func TestConditionAndOr(t *testing.T) {
	resolver := mapResolver(map[string]any{"体力": 15, "饱食": 50})
	checkCondition(t, resolver, "体力 < 20 ∧ 饱食 > 30", true)
	checkCondition(t, resolver, "体力 > 100 ∨ 饱食 < 60", true)
	checkCondition(t, resolver, "体力 > 100 ∧ 饱食 > 30", false)
}

func TestConditionNot(t *testing.T) {
	resolver := mapResolver(map[string]any{"体力": 15})
	checkCondition(t, resolver, "!体力 > 20", true)
	checkCondition(t, resolver, "~体力 == 30", true)
}

func TestConditionParens(t *testing.T) {
	resolver := mapResolver(map[string]any{"体力": 15, "饱食": 50, "水分": 90})
	checkCondition(t, resolver, "(体力 < 20 ∧ 饱食 > 30) ∨ (水分 < 10)", true)
}

func TestConditionStringCompare(t *testing.T) {
	resolver := mapResolver(map[string]any{"状态": "疲惫"})
	checkCondition(t, resolver, "状态 == '疲惫'", true)
	checkCondition(t, resolver, `状态 = "疲惫"`, true)
}

func TestConditionInSet(t *testing.T) {
	resolver := mapResolver(map[string]any{"位置": "客厅"})
	checkCondition(t, resolver, "位置 in {客厅,卧室}", true)
	checkCondition(t, resolver, "位置 in {厨房,浴室}", false)
}

func TestConditionContains(t *testing.T) {
	resolver := mapResolver(map[string]any{"事件": "猫猫打翻了水杯"})
	checkCondition(t, resolver, "事件 contains 打翻", true)
}

func TestConditionNonexistentPathFalse(t *testing.T) {
	resolver := mapResolver(map[string]any{"体力": 15})
	checkCondition(t, resolver, "不存在字段 > 10", false)
}

func TestConditionOrNotSplit(t *testing.T) {
	resolver := mapResolver(map[string]any{"行为": "oral_giving"})
	checkCondition(t, resolver, "行为 contains oral", true)
}

func TestConditionRightSidePath(t *testing.T) {
	data := map[string]any{
		"猫猫": map[string]any{"位置": "客厅", "状态": "活跃"},
		"梦梦": map[string]any{"位置": "客厅"},
		"场景": map[string]any{"事件": "客厅"},
	}
	resolver := func(path string) (any, error) {
		if strings.HasPrefix(path, "$") {
			varName := strings.Split(path, ".")[0]
			rest := strings.TrimPrefix(strings.TrimPrefix(path, varName), ".")
			entityIndex := int(varName[1] - '1')
			keys := []string{"猫猫", "梦梦"}
			target := data[keys[entityIndex]].(map[string]any)
			if rest == "" {
				return target, nil
			}
			return mapResolver(target)(rest)
		}
		return mapResolver(data)(path)
	}
	checkCondition(t, resolver, "$1.位置 = $2.位置", true)
	checkCondition(t, resolver, "场景.事件 = 猫猫.位置", true)
	checkCondition(t, resolver, "猫猫.状态 = 活跃", true)
	checkCondition(t, resolver, "猫猫.状态 = 疲惫", false)
}

func TestCompileConditionRejectsUnbalancedParens(t *testing.T) {
	if _, err := CompileCondition("(体力 < 20"); err == nil {
		t.Fatal("未闭合括号应返回错误")
	}
	if _, err := CompileCondition("体力 <"); err == nil {
		t.Fatal("缺少右操作数应返回错误")
	}
}
