// implicitInheritance_test.go locks the rule that colons in a type name are
// namespaces only: fields come from the explicit parent list, never from the
// name prefix.
package entity

import "testing"

func TestColonNameDoesNotInheritFields(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("父", map[string]any{"值": float64(1)})

	if order := store.ResolveOrder("父:子"); len(order) != 1 || order[0] != "父:子" {
		t.Fatalf("order = %v，冒号不应产生隐式父", order)
	}
	ent, err := store.Create("子", "父:子", nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got, err := ent.Get("值"); err == nil {
		t.Fatalf("值 = %v，未显式组合父模板时不应拿到父字段", got)
	}

	// 显式父列表仍然合并字段。
	store.SetTemplateParents("父:子", []string{"父"})
	if order := store.ResolveOrder("父:子"); len(order) != 2 || order[0] != "父" {
		t.Fatalf("explicit order = %v", order)
	}
	ent2, err := store.Create("子2", "父:子", nil, nil)
	if err != nil {
		t.Fatalf("create with parents: %v", err)
	}
	if got, _ := ent2.Get("值"); got != float64(1) {
		t.Fatalf("值 = %v，显式父应被合并", got)
	}
}

func TestMissingExplicitParentIsAnError(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("父:子", map[string]any{})
	store.SetTemplateParents("父:子", []string{"不存在"})
	if _, err := store.Create("子", "父:子", nil, nil); err == nil {
		t.Fatal("显式父不存在时必须报错")
	}
}
