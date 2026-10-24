package engine

import "testing"

func TestArithmeticConditionOperands(t *testing.T) {
	resolver := func(path string) (any, error) {
		if path == "变化量" {
			return float64(-25), nil
		}
		return nil, nil
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"变化量 + 1 > 0", false},
		{"变化量 * 2 < -40", true},
		{"(1 + 2) * 3 == 9", true},
		{"变化量 % 2 == 0", false},
	}
	for _, testCase := range cases {
		got, err := EvaluateCondition(testCase.expr, resolver)
		if err != nil {
			t.Fatalf("%s: %v", testCase.expr, err)
		}
		if got != testCase.want {
			t.Fatalf("%s = %v, want %v", testCase.expr, got, testCase.want)
		}
	}
}
