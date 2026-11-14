// clampInheritance_test.go verifies that numeric clamping inherits min/max
// field-by-field along the template chain.
package engine

import (
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
)

func TestClampInheritsAndOverridesRange(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("父", map[string]any{
		"值": float64(1),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int", "min": float64(0), "max": float64(100)},
		},
	})
	store.AddTemplate("父:子", map[string]any{
		"值": float64(2),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int", "min": float64(10), "max": float64(90)},
		},
	})
	ent, err := store.Create("测试实体", "父:子", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	if got := machine.clampValue(ent, "值", float64(5)); got != float64(10) {
		t.Fatalf("low clamp = %v, want 10", got)
	}
	if got := machine.clampValue(ent, "值", float64(95)); got != float64(90) {
		t.Fatalf("high clamp = %v, want 90", got)
	}
}

func TestClampInheritsParentRangeWhenChildHasNone(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("父", map[string]any{
		"值": float64(1),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int", "min": float64(0), "max": float64(100)},
		},
	})
	store.AddTemplate("父:子", map[string]any{
		"值": float64(2),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int"},
		},
	})
	ent, err := store.Create("测试实体", "父:子", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	if got := machine.clampValue(ent, "值", float64(200)); got != float64(100) {
		t.Fatalf("inherited high clamp = %v, want 100", got)
	}
}
