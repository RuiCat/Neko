// sceneOccupancyBoard_test.go proves the multi-agent contract end to end: one
// NPC moving writes 在场 on both scenes, the board records those changes and the
// main-session push therefore carries a 场景变化（<scene>）section.
package eventboard

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
)

// newOccupancyFixture builds two scenes and one NPC in S001-LR.
func newOccupancyFixture(t *testing.T) *entity.Store {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("场景", map[string]any{"事件": []any{}})
	for _, sceneID := range []string{"S001-LR", "S001-BR"} {
		if _, err := store.Create(sceneID, "场景", map[string]any{"事件": []any{}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Create("猫猫", "NPC", map[string]any{
		"位置": map[string]any{"场景": "S001-LR"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return store
}

// boardEntryOf returns one path's entry on a board.
func boardEntryOf(board *Board, path string) *Entry {
	if board == nil {
		return nil
	}
	for index := range board.Entries {
		if board.Entries[index].Path == path {
			return &board.Entries[index]
		}
	}
	return nil
}

// TestEndToEndSceneOccupancyReachesBoard follows one move across both boards.
func TestEndToEndSceneOccupancyReachesBoard(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	store := newOccupancyFixture(t)
	machine := engine.New(store)
	registry := New(filepath.Join(t.TempDir(), "blackboard.json"))

	if err := registry.Apply(machine.Tick("*")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	entry := boardEntryOf(registry.Board("S001-LR"), "在场")
	if entry == nil || !valuesEqual(entry.To, []any{"猫猫"}) {
		t.Fatalf("S001-LR 在场 = %+v, want [猫猫]", entry)
	}

	if err := store.Get("猫猫").Set(config.Current().ScenePath(), "S001-BR"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Apply(machine.Tick("*")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	arrive := boardEntryOf(registry.Board("S001-BR"), "在场")
	if arrive == nil || !valuesEqual(arrive.To, []any{"猫猫"}) {
		t.Fatalf("S001-BR 在场 = %+v, want [猫猫]", arrive)
	}
	left := boardEntryOf(registry.Board("S001-LR"), "在场")
	if left == nil || !valuesEqual(left.From, []any{"猫猫"}) || !valuesEqual(left.To, []any{}) {
		t.Fatalf("S001-LR 在场 = %+v, want [猫猫] -> []", left)
	}
}
