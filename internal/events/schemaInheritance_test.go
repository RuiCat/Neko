// schemaInheritance_test.go verifies that a child annotation inherits unset
// constraints from its parent template.
package events

import (
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
)

func TestSchemaInheritsUnsetParentConstraints(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("父", map[string]any{
		"值": float64(1),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int", "event": "值事件", "min": float64(0), "max": float64(100)},
		},
	})
	store.AddTemplate("父:子", map[string]any{
		"值": float64(2),
		"__annotations__": map[string]any{
			"值": map[string]any{"type": "int"},
		},
	})
	bridge := NewBridge(store, nil)
	schema := bridge.schemaFor("父:子")
	if schema == nil {
		t.Fatal("schema missing")
	}
	field := schema.Get("值")
	if field == nil {
		t.Fatal("field schema missing")
	}
	if field.Type != "int" || field.Event != "值事件" {
		t.Fatalf("inherited field = %+v", field)
	}
	if field.Min == nil || *field.Min != 0 || field.Max == nil || *field.Max != 100 {
		t.Fatalf("inherited range = %v..%v", field.Min, field.Max)
	}
}
