package function

import "testing"

func TestMapLiteral(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `映射 = {"a": 1, "b": 2}
返回 映射["a"] + 映射.get("c", 10)`
	if got := registerAndCall(t, executor, body, nil, nil); got != 11 {
		t.Fatalf("map literal = %v", got)
	}
}

func TestListMethods(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `列表 = [3, 1, 2]
列表.append(4)
列表.extend([5, 6])
列表.insert(0, 0)
列表.remove(3)
弹出 = 列表.pop()
返回 列表.count(2) + 列表.index(1) + 弹出`
	if got := registerAndCall(t, executor, body, nil, nil); got != 8 {
		t.Fatalf("list methods = %v", got)
	}
}

func TestTextAndMapMethods(t *testing.T) {
	executor := newBoundaryExecutor(t)
	textBody := `文本 = " AbC "
返回 文本.strip().lower().replace("b", "x")`
	if got := registerAndCall(t, executor, textBody, nil, nil); got != "axc" {
		t.Fatalf("text methods = %v", got)
	}
	joinBody := `返回 ",".join(["a", "b"])`
	if got := registerAndCall(t, executor, joinBody, nil, nil); got != "a,b" {
		t.Fatalf("join = %v", got)
	}
	mapBody := `映射 = {"b": 2, "a": 1}
返回 ", ".join(映射.keys())`
	if got := registerAndCall(t, executor, mapBody, nil, nil); got != "a, b" {
		t.Fatalf("map keys = %v", got)
	}
	splitBody := `返回 "a,b,c".split(",")[1]`
	if got := registerAndCall(t, executor, splitBody, nil, nil); got != "b" {
		t.Fatalf("split = %v", got)
	}
}

func TestMoreMethods(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`列表 = [3, 1, 2]
列表.sort()
返回 列表[0]`, "1"},
		{`映射 = {"a": 1}
映射.update({"b": 2})
返回 映射["b"]`, "2"},
		{`映射 = {"a": 1}
返回 映射.setdefault("b", 5)`, "5"},
		{`返回 "abc".find("b")`, "1"},
		{`返回 "abcabc".rfind("b")`, "4"},
		{`返回 "hello world".title()`, "Hello World"},
		{`返回 "abc".isdigit()`, "False"},
		{`返回 "123".isdigit()`, "True"},
		{`返回 [1] + [2]`, "[1, 2]"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); pyText(got) != testCase.want {
			t.Fatalf("%q = %v, want %s", testCase.body, pyText(got), testCase.want)
		}
	}
}

func TestExtendedTextAndDictMethods(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`返回 "7".zfill(3)`, "007"},
		{`返回 "a,b,c".rsplit(",", 1)[1]`, "c"},
		{`返回 "abc".partition("b")[1]`, "b"},
		{`返回 "abcabc".count("bc")`, "2"},
		{`返回 "abc".islower()`, "True"},
		{`返回 "  ".isspace()`, "True"},
		{`返回 "ab1".isalnum()`, "True"},
		{`映射 = {"a": 1, "b": 2}
配对 = 映射.popitem()
返回 配对[0]`, "b"},
		{`映射 = {"a": 1}
返回 映射.pop("a")`, "1"},
		{`try { "abc".index("z") } except 值错误 as e { 返回 "没找到" }`, "没找到"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); pyText(got) != testCase.want {
			t.Fatalf("%q = %v, want %s", testCase.body, pyText(got), testCase.want)
		}
	}
}

func TestRemainingTextMethods(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`返回 "Ab Cd".istitle()`, "True"},
		{`返回 "ab".istitle()`, "False"},
		{`返回 "abc".isascii()`, "True"},
		{`返回 "喵".isascii()`, "False"},
		{`返回 "ab".isprintable()`, "True"},
		{`返回 "a\tb".expandtabs(4)`, "a   b"},
		{`返回 "AbC".casefold()`, "abc"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); pyText(got) != testCase.want {
			t.Fatalf("%q = %v, want %s", testCase.body, pyText(got), testCase.want)
		}
	}
}

func TestFormatMethod(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`返回 "{}-{}".format(1, "a")`, "1-a"},
		{`返回 "{0}{0}".format("x")`, "xx"},
		{`返回 "{{}}".format()`, "{}"},
		{`返回 "{1}-{0}".format("a", "b")`, "b-a"},
		{`返回 "{:.2f}".format(1.234)`, "1.23"},
		{`返回 "{:>5}".format("a")`, "    a"},
		{`返回 "{:05d}".format(42)`, "00042"},
		{`返回 "{:^7}".format("ab")`, "  ab   "},
		{`返回 "{!r}".format("x")`, "'x'"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); got != testCase.want {
			t.Fatalf("%q = %v, want %s", testCase.body, got, testCase.want)
		}
	}
}
