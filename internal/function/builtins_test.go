package function

import "testing"

func TestListComprehension(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `返回 [x * x for x in [1, 2, 3, 4] if x % 2 == 0]`
	if got := registerAndCall(t, executor, body, nil, nil); pyText(got) != "[4, 16]" {
		t.Fatalf("comprehension = %v", pyText(got))
	}
}

func TestExtraBuiltins(t *testing.T) {
	executor := newBoundaryExecutor(t)
	cases := []struct {
		body string
		want string
	}{
		{`返回 len(range(1, 6))`, "5"},
		{`返回 len(enumerate([1, 2, 3]))`, "3"},
		{`返回 len(zip([1, 2], [3, 4, 5]))`, "2"},
		{`返回 sum(range(1, 101))`, "5050"},
		{`返回 sorted([3, 1, 2])[0]`, "1"},
		{`返回 reversed("abc")`, "cba"},
	}
	for _, testCase := range cases {
		if got := registerAndCall(t, executor, testCase.body, nil, nil); pyText(got) != testCase.want {
			t.Fatalf("%s = %v, want %s", testCase.body, pyText(got), testCase.want)
		}
	}
}

func TestRandomBuiltin(t *testing.T) {
	executor := newBoundaryExecutor(t)
	body := `
样本 = []
for i in range(0, 30) {
  样本.append(随机(1, 3))
}
返回 [样本, 随机(5, 1), 随机(-2, -2), 随机(0, 0)]`
	got := registerAndCall(t, executor, body, nil, nil)
	parts, ok := got.([]any)
	if !ok || len(parts) != 4 {
		t.Fatalf("返回 = %#v", got)
	}
	samples, ok := parts[0].([]any)
	if !ok || len(samples) != 30 {
		t.Fatalf("样本 = %#v", parts[0])
	}
	seen := map[int]bool{}
	for _, sample := range samples {
		number, ok := sample.(int)
		if !ok || number < 1 || number > 3 {
			t.Fatalf("随机(1,3) 越界: %#v", sample)
		}
		seen[number] = true
	}
	if len(seen) != 3 {
		t.Fatalf("随机(1,3) 只出现 %v", seen)
	}
	if number, ok := parts[1].(int); !ok || number < 1 || number > 5 {
		t.Fatalf("随机(5,1) 越界: %#v", parts[1])
	}
	if parts[2] != -2 || parts[3] != 0 {
		t.Fatalf("同界随机 = %#v / %#v", parts[2], parts[3])
	}
}
