// annotation_test.go covers template metadata hygiene and per-field annotation
// inheritance.
package entity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateEntityStripsTemplateMetadata(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("测试类型", map[string]any{
		"值":               float64(1),
		"__annotations__": map[string]any{"值": map[string]any{"type": "int"}},
	})
	ent, err := store.Create("测试实体", "测试类型", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ent.Snapshot()
	if _, ok := snapshot["__annotations__"]; ok {
		t.Fatal("__annotations__ leaked into instance data")
	}
	if _, ok := snapshot["__parents__"]; ok {
		t.Fatal("__parents__ leaked into instance data")
	}
}

func TestLoadStripsTemplateMetadata(t *testing.T) {
	dir := t.TempDir()
	payload := `{
  "version": 1,
  "entities": {
    "旧实体": {
      "type": "测试类型",
      "data": {"值": 1, "__annotations__": {"值": {"type": "int"}}},
      "keys": {},
      "_meta": {}
    }
  }
}`
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entities.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	ent := store.Get("旧实体")
	if ent == nil {
		t.Fatal("entity not loaded")
	}
	if _, ok := ent.Snapshot()["__annotations__"]; ok {
		t.Fatal("__annotations__ was not stripped during load")
	}
}

func TestFieldSchemaMergeInherited(t *testing.T) {
	parent := &FieldSchema{Type: "int", Event: "事件", Min: floatPtr(0), Max: floatPtr(100)}
	child := &FieldSchema{Type: ""}
	child.MergeInherited(parent)
	if child.Type != "int" || child.Event != "事件" || child.Min == nil || child.Max == nil {
		t.Fatalf("merged schema = %+v", child)
	}
}

func floatPtr(value float64) *float64 { return &value }
