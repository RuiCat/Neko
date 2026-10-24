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
