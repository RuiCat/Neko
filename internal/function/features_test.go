// Syntax feature tests for constructs the corpus does not exercise directly:
// not/and/or operators, list building with for+append and augmented assignment.
package function

import (
	"reflect"
	"testing"

	"github.com/ruicat/astrbox/internal/neko"
)

// registerInline registers one ad-hoc function and fails the test on errors.
func registerInline(t *testing.T, executor *Executor, name, body string, params ...string) {
	t.Helper()
	definition := neko.FuncDef{Name: name, Body: body}
	for _, param := range params {
		definition.Params = append(definition.Params, neko.Param{Name: param})
	}
	if err := executor.Register(definition); err != nil {
		t.Fatalf("注册 %s 失败: %v", name, err)
	}
}

// TestInterpreterBooleanOperators covers !/not and and/or/∧/∨.
func TestInterpreterBooleanOperators(t *testing.T) {
	executor := New(nil)
	registerInline(t, executor, "逻辑", `返回 (not false) ∧ (true or false) ∧ (1 < 2)`)
	got, err := executor.Call("逻辑", nil, nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if got != true {
		t.Fatalf("逻辑 = %#v, 期望 true", got)
	}

	registerInline(t, executor, "取反", `返回 !true`)
	got, err = executor.Call("取反", nil, nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if got != false {
		t.Fatalf("取反 = %#v, 期望 false", got)
	}
}

// TestInterpreterLoopAppend covers for + append + augmented assignment.
func TestInterpreterLoopAppend(t *testing.T) {
	executor := New(nil)
	registerInline(t, executor, "复制", "结果 = []\nfor 项 in 列表 {\n  结果.append(项)\n}\n返回 结果", "列表")
	got, err := executor.Call("复制", []any{[]any{"a", "b"}}, nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("复制 = %#v", got)
	}

	registerInline(t, executor, "求和", "合计 = 0\nfor 项 in 列表 {\n  合计 += 项\n}\n返回 合计", "列表")
	got, err = executor.Call("求和", []any{[]any{1, 2, 3}}, nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if got != 6 {
		t.Fatalf("求和 = %#v, 期望 6", got)
	}
}

// TestInterpreterSlices covers string indexing, slicing and int().
func TestInterpreterSlices(t *testing.T) {
	executor := New(nil)
	registerInline(t, executor, "小时", `返回 int(文本[11:13]) + int(文本[0:1])`, "文本")
	got, err := executor.Call("小时", []any{"2026-08-05T17:30:00+08:00"}, nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if got != 19 {
		t.Fatalf("小时 = %#v, 期望 19", got)
	}
}
