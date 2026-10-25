package entity

import (
	"sync"
	"testing"
)

func TestSnapshotDeepCopiesData(t *testing.T) {
	ent := &Entity{ID: "猫猫", Type: "NPC", Data: map[string]any{
		"身体": map[string]any{"体力": 50},
		"标签": []any{"a"},
	}}

	snapshot := ent.Snapshot()
	snapshot["身体"].(map[string]any)["体力"] = 1
	snapshot["标签"].([]any)[0] = "b"

	if got, err := ent.Get("身体.体力"); err != nil || got != 50 {
		t.Fatalf("snapshot mutated entity data: got %v, err %v", got, err)
	}
	if got := ent.GetOrNone("标签").([]any)[0]; got != "a" {
		t.Fatalf("snapshot shares list with entity: got %v", got)
	}
}

func TestSnapshotConcurrentWithSetAndSave(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	ent, err := store.Create("猫猫", "NPC", map[string]any{"身体": map[string]any{"体力": 50}}, nil)
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}

	const rounds = 100
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			if err := ent.Set("身体.体力", i); err != nil {
				t.Errorf("set: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			if err := store.Save(); err != nil {
				t.Errorf("save: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			if snapshot := ent.Snapshot(); snapshot == nil {
				t.Error("snapshot is nil")
				return
			}
		}
	}()
	wait.Wait()
}
