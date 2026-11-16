// Tests for readonly annotation enforcement in Entity.Set.
package entity

import "testing"

// TestSetRejectsReadonlyField pins the readonly contract: writes to a field
// marked readonly in the template chain fail, other fields keep working.
func TestSetRejectsReadonlyField(t *testing.T) {
	ent := &Entity{
		ID:   "NPC:测试",
		Type: "NPC",
		Data: map[string]any{},
		Templates: map[string]map[string]any{
			"NPC": {"__annotations__": map[string]any{
				"状态.锁定": map[string]any{"readonly": true, "type": "int"},
				"状态.正常": map[string]any{"type": "int"},
			}},
		},
		Parents: map[string][]string{},
	}
	if err := ent.Set("状态.锁定", 1); err == nil {
		t.Fatal("readonly 字段写入未报错")
	}
	if err := ent.Set("状态.正常", 1); err != nil {
		t.Fatalf("普通字段写入失败: %v", err)
	}
	if got, _ := ent.Get("状态.正常"); got != 1 {
		t.Fatalf("普通字段 = %v", got)
	}
}
