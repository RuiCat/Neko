// Multi-subject "$N.路径" action-value tests: the multi resolver only accepts the
// "$N." form, so action values must not strip the leading "$" before resolving.
package engine

import "testing"

func TestActionValueMultiSubjectRefsResolve(t *testing.T) {
	store := newMultiStore(t)
	setPath(t, store, "猫猫", "身体.体力", 10.0)
	setPath(t, store, "梦梦", "身体.体力", 60.0)
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_multi_value", Match: "NPC[*]", Enabled: true,
		Subjects: []string{"NPC[*]", "NPC[*]"},
		Branches: []Branch{{
			If: stringPtr("$1.身体.体力 < 20 ∧ $2.身体.体力 > 50"),
			Then: []Op{
				{Op: "set", Subject: 1, Path: "标记.对方体力", Value: "$2.身体.体力"},
				{Op: "set", Subject: 2, Path: "标记.对方体力", Value: "$1.身体.体力"},
			},
		}},
	})
	result := machine.Tick("")
	if result.Status != "ok" {
		t.Fatalf("status = %s, 期望 ok: %v", result.Status, result.Errors)
	}
	expectPath(t, store, "猫猫", "标记.对方体力", 60.0)
	expectPath(t, store, "梦梦", "标记.对方体力", 10.0)
}

func TestActionValueMultiSubjectRefMissingKeepsLiteral(t *testing.T) {
	store := newMultiStore(t)
	setPath(t, store, "猫猫", "身体.体力", 10.0)
	setPath(t, store, "梦梦", "身体.体力", 60.0)
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_multi_missing", Match: "NPC[*]", Enabled: true,
		Subjects: []string{"NPC[*]", "NPC[*]"},
		Branches: []Branch{{
			If: stringPtr("$1.身体.体力 < 20 ∧ $2.身体.体力 > 50"),
			Then: []Op{
				{Op: "set", Subject: 1, Path: "标记.缺失", Value: "$2.不存在.路径"},
			},
		}},
	})
	_ = machine.Tick("")
	expectPath(t, store, "猫猫", "标记.缺失", "$2.不存在.路径")
}
