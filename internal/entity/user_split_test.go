package entity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUserEntitiesSplit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "NPC:猫猫", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("场景", "场景", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	public := readEntityIDs(t, filepath.Join(dir, "entities.json"))
	if _, ok := public["猫猫"]; ok {
		t.Fatal("NPC entity leaked into public entities.json")
	}
	if _, ok := public["场景"]; !ok {
		t.Fatal("scene missing from public entities.json")
	}
	user := readEntityIDs(t, filepath.Join(dir, "user", "entities.json"))
	if _, ok := user["猫猫"]; !ok {
		t.Fatal("NPC entity missing from user/entities.json")
	}

	reloaded := NewStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.Get("猫猫") == nil || reloaded.Get("场景") == nil {
		t.Fatal("reload did not merge public + user entities")
	}
}

func TestMigrateUserEntitiesFromLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":1,"entities":{"猫猫":{"id":"猫猫","type":"NPC:猫猫","keys":{},"data":{}}}}`
	if err := os.WriteFile(filepath.Join(dir, "entities.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateUserEntities(); err != nil {
		t.Fatal(err)
	}
	user := readEntityIDs(t, filepath.Join(dir, "user", "entities.json"))
	if _, ok := user["猫猫"]; !ok {
		t.Fatal("migration did not move NPC entity")
	}
}

func readEntityIDs(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var payload struct {
		Entities map[string]any `json:"entities"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return payload.Entities
}
