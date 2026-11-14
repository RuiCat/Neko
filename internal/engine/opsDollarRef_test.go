// Action-value "$" reference tests: "$.路径", "$路径" and "{e}.路径" must all
// resolve to the current entity field instead of writing a literal back.
package engine

import "testing"

func TestActionValueDollarRefsResolveCurrentEntity(t *testing.T) {
	store := newTestStore(t)
	setPath(t, store, "猫猫", "循环.灌注", 94)
	machine := New(store)
	empty := ""
	cases := []struct {
		id     string
		value  string
		target string
	}{
		{"r_dollar_dot", "$.循环.灌注", "标记.点路径"},
		{"r_dollar_bare", "$循环.灌注", "标记.裸路径"},
		{"r_dollar_brace", "{e}.循环.灌注", "标记.花括号路径"},
	}
	for _, tc := range cases {
		machine.AddRule(Rule{
			ID: tc.id, Match: "NPC[*]", Enabled: true,
			Branches: []Branch{{
				If:   &empty,
				Then: []Op{{Op: "set", Path: "{e}." + tc.target, Value: tc.value}},
			}},
		})
	}
	result := machine.Tick("猫猫")
	if result.Status != "ok" {
		t.Fatalf("status = %s, 期望 ok: %v", result.Status, result.Errors)
	}
	for _, tc := range cases {
		expectPath(t, store, "猫猫", tc.target, 94)
	}
}
