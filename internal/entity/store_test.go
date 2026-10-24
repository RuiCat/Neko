package entity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRealData(t *testing.T) {
	store := NewStore("../../data")
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if store.Count() == 0 {
		t.Fatal("expected entities")
	}
	if len(store.ListTemplates()) == 0 {
		t.Fatal("expected templates")
	}
	cat := store.Get("猫猫")
	if cat == nil {
		t.Fatal("entity 猫猫 missing")
	}
	if cat.Type != "NPC:猫猫" {
		t.Fatalf("unexpected type: %s", cat.Type)
	}
	value, err := cat.Get("身体.体力")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value == nil {
		t.Fatal("体力 is nil")
	}
}

func TestQueryAndTemplates(t *testing.T) {
	store := NewStore("../../data")
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	entities, err := store.Query("NPC[*]")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected NPC entities")
	}
	if _, err := store.Query("猫猫"); err != nil {
		t.Fatalf("query by id: %v", err)
	}
	if !store.HasTemplate("NPC") {
		t.Fatal("template NPC missing")
	}
}

func TestCreateEntityWithInheritance(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := `{"身体":{"体力":50},"状态":{"身体":"活跃"}}`
	if err := os.WriteFile(filepath.Join(dir, "templates", "NPC.json"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	ent, err := store.Create("小黑", "NPC", map[string]any{"身体": map[string]any{"体力": float64(80)}}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	value, err := ent.Get("身体.体力")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != float64(80) {
		t.Fatalf("override failed: %v", value)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded := NewStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Get("小黑") == nil {
		t.Fatal("entity not persisted")
	}
}
