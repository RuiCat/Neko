// itemsView_test.go pins the 素材.物品 projection and the snapshot precedence: the
// item view is live and ordered, a native entity field wins, and a malformed
// material surfaces as an error instead of a silent empty view.
package host

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
)

// TestItemsViewProjectsMaterialEntities asserts the projection rows, their
// ordering and the malformed-row behavior.
func TestItemsViewProjectsMaterialEntities(t *testing.T) {
	store, _ := newTestStore(t, npcTemplate)
	if rows, err := NewItemsView(store).View(); err != nil || len(rows) != 0 {
		t.Fatalf("empty view = %v, %v", rows, err)
	}
	if _, err := NewItemsView(nil).View(); err == nil {
		t.Fatal("nil store produced an empty view instead of an error")
	}

	createMaterial(t, store, "食材@S001-KT", "食材", testScene, 2)
	createMaterial(t, store, "食材@S008", "食材", testOtherScene, 3)
	rope := createMaterial(t, store, "绳子", "绳子", "", 1)
	rows, err := NewItemsView(store).View()
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	wantOrder := []string{"绳子", "食材", "食材"}
	for index, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("row %d: got %T", index, raw)
		}
		if name := row[condition.KEY_ITEM_NAME]; name != wantOrder[index] {
			t.Fatalf("row %d name = %v, want %q", index, name, wantOrder[index])
		}
		if _, ok := row[condition.KEY_ITEM_QUANTITY].(int); !ok {
			t.Fatalf("row %d quantity is %T, want int", index, row[condition.KEY_ITEM_QUANTITY])
		}
	}
	first := rows[0].(map[string]any)
	if scene := first[condition.KEY_ITEM_SCENE]; scene != "" {
		t.Fatalf("unscoped material scene = %v, want empty", scene)
	}

	if err := rope.Set(condition.KEY_ITEM_QUANTITY, "一颗"); err != nil {
		t.Fatalf("corrupt quantity: %v", err)
	}
	if _, err := NewItemsView(store).View(); err == nil {
		t.Fatal("malformed 数量 did not surface as an error")
	}
}

// TestSnapshotResolvesEntityAndItemPaths asserts the K2 view reads entity paths,
// projects the live item view, and prefers a native 素材.物品 field.
func TestSnapshotResolvesEntityAndItemPaths(t *testing.T) {
	store, _ := newTestStore(t, npcTemplate)
	actor := createActor(t, store)
	createMaterial(t, store, "食材@S001-KT", "食材", testScene, 2)
	hostStore := NewEntityStore(store)

	snapshot, err := hostStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	scene, err := snapshot.Get(condition.PATH_SCENE)
	if err != nil || scene != testScene {
		t.Fatalf("scene = %v, %v", scene, err)
	}
	items, err := snapshot.Get(condition.PATH_ITEMS)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	rows, ok := items.([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("items = %#v", items)
	}

	native := []any{map[string]any{
		condition.KEY_ITEM_NAME: "手写", condition.KEY_ITEM_SCENE: "S999",
		condition.KEY_ITEM_QUANTITY: 3,
	}}
	if err := actor.Set(condition.PATH_ITEMS, native); err != nil {
		t.Fatalf("native item field: %v", err)
	}
	items, err = snapshot.Get(condition.PATH_ITEMS)
	if err != nil {
		t.Fatalf("native items: %v", err)
	}
	preferred, ok := items.([]any)
	if !ok || len(preferred) != 1 {
		t.Fatalf("native items = %#v", items)
	}
	row := preferred[0].(map[string]any)
	if row[condition.KEY_ITEM_NAME] != "手写" {
		t.Fatalf("snapshot did not prefer the native field: %#v", row)
	}

	if _, err := hostStore.Snapshot("不存在"); err == nil {
		t.Fatal("snapshot of a missing entity did not fail")
	}
}
