package function

import (
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

func newBoundaryExecutor(t *testing.T) *Executor {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{"设置": map[string]any{"a": 1, "b": 2}})
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatal(err)
	}
	return New(store)
}

func registerAndCall(t *testing.T, executor *Executor, body string, params []string, args []any) any {
	t.Helper()
	declared := make([]neko.Param, len(params))
	for index, name := range params {
		declared[index] = neko.Param{Name: name}
	}
	def := neko.FuncDef{Name: "测试", Params: declared, Body: body}
	if err := executor.Register(def); err != nil {
		t.Fatalf("register: %v", err)
	}
	value, err := executor.Call("测试", args, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return value
}

func TestChainedComparison(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `if 1 < a < b < c {
  返回 true
}
返回 false`
	if got := registerAndCall(t, executor, body, []string{"a", "b", "c"}, []any{2, 3, 4}); got != true {
		t.Fatalf("1<2<3<4 = %v", got)
	}
	if got := registerAndCall(t, executor, body, []string{"a", "b", "c"}, []any{3, 2, 1}); got != false {
		t.Fatalf("1<3<2<1 = %v", got)
	}
}

func TestWhileLoop(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `总数 = 0
i = 1
while i <= 5 {
  总数 = 总数 + i
  i = i + 1
}
返回 总数`
	if got := registerAndCall(t, executor, body, nil, nil); got != 15 {
		t.Fatalf("while sum = %v", got)
	}
}

func TestForOverStringAndMap(t *testing.T) {
	executor := newBoundaryExecutor(t)
	stringBody := `结果 = ""
for 字符 in 文本 {
  结果 = 结果 + 字符
}
返回 结果`
	if got := registerAndCall(t, executor, stringBody, []string{"文本"}, []any{"喵喵"}); got != "喵喵" {
		t.Fatalf("string for = %v", got)
	}
	mapBody := `数据 = e.设置
结果 = ""
for 键 in 数据 {
  结果 = 结果 + 键
}
返回 结果`
	if got := registerAndCall(t, executor, mapBody, []string{"e"}, []any{"猫猫"}); got != "ab" {
		t.Fatalf("map for = %v", got)
	}
}

func TestSliceStep(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `返回 列表[low:high:step]`
	cases := []struct {
		low, high, step any
		want            string
	}{
		{nil, nil, 2, "[0, 2, 4]"},
		{nil, nil, -1, "[4, 3, 2, 1, 0]"},
		{1, 4, 2, "[1, 3]"},
	}
	for _, testCase := range cases {
		got := registerAndCall(t, executor, body, []string{"列表", "low", "high", "step"}, []any{[]any{0, 1, 2, 3, 4}, testCase.low, testCase.high, testCase.step})
		if gotText := pyText(got); gotText != testCase.want {
			t.Fatalf("slice(%v,%v,%v) = %s, want %s", testCase.low, testCase.high, testCase.step, gotText, testCase.want)
		}
	}
}

func TestTryExcept(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `try {
  未知函数()
} except 错误 {
  返回 错误
}`
	got := pyText(registerAndCall(t, executor, body, nil, nil))
	if !strings.Contains(got, "未定义") {
		t.Fatalf("try/except = %q", got)
	}
}

func TestForTupleUnpacking(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `结果 = ""
for 序号, 字 in enumerate(["a", "b"]) {
  结果 = 结果 + str(序号) + 字
}
返回 结果`
	if got := registerAndCall(t, executor, body, nil, nil); got != "0a1b" {
		t.Fatalf("unpack = %v", got)
	}
}

func TestStrictPythonTypes(t *testing.T) {
	executor := newBoundaryExecutor(t)
	intBody := `try {
  int("abc")
} except 错误 {
  返回 "转换失败"
}`
	if got := registerAndCall(t, executor, intBody, nil, nil); got != "转换失败" {
		t.Fatalf("int strictness = %v", got)
	}
	addBody := `try {
  "x" + [1]
} except 错误 {
  返回 "类型错误"
}`
	if got := registerAndCall(t, executor, addBody, nil, nil); got != "类型错误" {
		t.Fatalf("+ strictness = %v", got)
	}
	if got := registerAndCall(t, executor, `返回 [1] + [2]`, nil, nil); pyText(got) != "[1, 2]" {
		t.Fatalf("list + list = %v", pyText(got))
	}
	if got := registerAndCall(t, executor, `返回 "ab" * 2`, nil, nil); got != "abab" {
		t.Fatalf("text * int = %v", got)
	}
}

func TestTupleAssignment(t *testing.T) {
	executor := newBoundaryExecutor(t)
	if got := registerAndCall(t, executor, "a, b = 1, 2\n返回 a + b", nil, nil); got != 3 {
		t.Fatalf("tuple assign = %v", got)
	}
	if got := registerAndCall(t, executor, "对 = [3, 4]\nx, y = 对\n返回 y - x", nil, nil); got != 1 {
		t.Fatalf("tuple unpack = %v", got)
	}
}

func TestBreakContinue(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `总数 = 0
for 数字 in range(1, 10) {
  if 数字 % 2 == 0 {
    continue
  }
  if 数字 > 6 {
    break
  }
  总数 = 总数 + 数字
}
返回 总数`
	if got := registerAndCall(t, executor, body, nil, nil); got != 9 {
		t.Fatalf("break/continue = %v", got)
	}
}

func TestTryFinally(t *testing.T) {
	executor := newBoundaryExecutor(t)
	override := `try {
  返回 1
} finally {
  返回 2
}`
	if got := registerAndCall(t, executor, override, nil, nil); got != 2 {
		t.Fatalf("finally return override = %v", got)
	}
	sideEffect := `try {
  设置.a = 1
} finally {
  设置.a = 2
}
返回 设置.a`
	def := neko.FuncDef{Name: "副作用", Body: sideEffect}
	if err := executor.Register(def); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := executor.Call("副作用", nil, executor.store.Get("猫猫"))
	if err != nil || got != 2 {
		t.Fatalf("finally side effect = %v, %v", got, err)
	}
}

func TestTypedExcept(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`try { int("x") } except 值错误 as e { 返回 "值" }`, "值"},
		{`try { "x" + [1] } except 类型错误 as e { 返回 "类型" }`, "类型"},
		{`try { 未知() } except 名称错误 as e { 返回 "名称" }`, "名称"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); got != testCase.want {
			t.Fatalf("%s = %v", testCase.body, got)
		}
	}
	def := neko.FuncDef{Name: "不匹配", Body: `try { int("x") } except 类型错误 as e { 返回 "类型" }`}
	if err := executor.Register(def); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Call("不匹配", nil, nil); err == nil {
		t.Fatal("mismatched except type must propagate the error")
	}
}
