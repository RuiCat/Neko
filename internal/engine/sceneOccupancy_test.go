// sceneOccupancy_test.go covers the per-tick scene occupancy reconciliation:
// one NPC moving updates both scenes' 在场 lists and reports the change as a
// tick write (which the event board turns into a pushed scene change).
package engine

import (
	"testing"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// newOccupancyStore builds two scenes and two NPCs sharing S001-LR.
func newOccupancyStore(t *testing.T) *entity.Store {
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
	for _, npcID := range []string{"猫猫", "梦梦"} {
		if _, err := store.Create(npcID, "角色基础", map[string]any{
			"位置": map[string]any{"场景": "S001-LR"},
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

// occupants reads one scene's 在场 list as strings ("" when absent).
func occupants(t *testing.T, store *entity.Store, sceneID string) []string {
	t.Helper()
	scene := store.Get(sceneID)
	if scene == nil {
		t.Fatalf("场景 %s 不存在", sceneID)
	}
	raw, _ := scene.GetOrNone("在场").([]any)
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		ids = append(ids, item.(string))
	}
	return ids
}

// moveActor writes the actor's current-scene field the way an external write
// (or a done tick) leaves it.
func moveActor(t *testing.T, store *entity.Store, npcID, sceneID string) {
	t.Helper()
	if err := store.Get(npcID).Set(config.Current().ScenePath(), sceneID); err != nil {
		t.Fatalf("移动 %s: %v", npcID, err)
	}
}

// TestSceneOccupancyTracksMovement is the user-visible contract: 猫猫 walking
// into S001-BR changes both scenes' 在场 and the tick reports those writes.
func TestSceneOccupancyTracksMovement(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	store := newOccupancyStore(t)
	machine := New(store)

	machine.Tick("*")
	if got := occupants(t, store, "S001-LR"); len(got) != 2 || got[0] != "梦梦" || got[1] != "猫猫" {
		t.Fatalf("S001-LR 在场 = %v, want [梦梦 猫猫]", got)
	}
	if got := store.Get("S001-BR").GetOrNone("在场"); got != nil {
		t.Fatalf("空场景不应凭空创建在场字段: %#v", got)
	}

	moveActor(t, store, "猫猫", "S001-BR")
	result := machine.Tick("*")
	if got := occupants(t, store, "S001-LR"); len(got) != 1 || got[0] != "梦梦" {
		t.Fatalf("S001-LR 在场 = %v, want [梦梦]", got)
	}
	if got := occupants(t, store, "S001-BR"); len(got) != 1 || got[0] != "猫猫" {
		t.Fatalf("S001-BR 在场 = %v, want [猫猫]", got)
	}
	left := findWrite(result, "S001-LR", "在场")
	if left == nil || !writeListEquals(left.From, []any{"梦梦", "猫猫"}) || !writeListEquals(left.To, []any{"梦梦"}) {
		t.Fatalf("S001-LR 在场写入 = %+v, want [梦梦 猫猫] -> [梦梦]", left)
	}
	arrive := findWrite(result, "S001-BR", "在场")
	if arrive == nil || !writeListEquals(arrive.To, []any{"猫猫"}) {
		t.Fatalf("S001-BR 在场写入 = %+v, want -> [猫猫]", arrive)
	}
}

// TestSceneOccupancyWritesOnlyOnChange keeps a quiet tick quiet.
func TestSceneOccupancyWritesOnlyOnChange(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	store := newOccupancyStore(t)
	machine := New(store)
	machine.Tick("*")
	result := machine.Tick("*")
	if write := findWrite(result, "S001-LR", "在场"); write != nil {
		t.Fatalf("位置没变时不应写在场: %+v", write)
	}
}

// TestSceneOccupancyDisabledByEmptyField covers scene_occupancy_field = "".
func TestSceneOccupancyDisabledByEmptyField(t *testing.T) {
	cfg := config.Default()
	cfg.SceneOccupancyField = ""
	if err := config.Set(cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)
	store := newOccupancyStore(t)
	New(store).Tick("*")
	if got := store.Get("S001-LR").GetOrNone("在场"); got != nil {
		t.Fatalf("关闭同步后不应写在场: %#v", got)
	}
}

// writeListEquals compares one write side with an expected id list.
func writeListEquals(raw any, want []any) bool {
	got, ok := raw.([]any)
	if !ok {
		return raw == nil && len(want) == 0
	}
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
