// snapshot_test.go covers the read-only condition.Snapshot adapter: flat dotted
// keys, nested objects, the 素材.物品 view and the error paths.
package settle

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
)

// TestSnapshotAdapterGet proves both flat dotted keys and nested objects
// resolve to the same value, which the runtime and JSON shapes require.
func TestSnapshotAdapterGet(t *testing.T) {
	flat := NewSnapshot(map[string]any{"代谢.胃内容": 800})
	nested := NewSnapshot(map[string]any{"代谢": map[string]any{"胃内容": 800}})
	for name, snap := range map[string]Snapshot{"flat": flat, "nested": nested} {
		t.Run(name, func(t *testing.T) {
			value, err := snap.Get("代谢.胃内容")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if value != 800 {
				t.Fatalf("value = %v, want 800", value)
			}
		})
	}
}

// TestSnapshotAdapterItems proves 素材.物品 is returned verbatim in both
// accepted shapes and that the decoder reads them identically.
func TestSnapshotAdapterItems(t *testing.T) {
	views := []map[string]any{
		{"素材.物品": []any{map[string]any{"名称": "食材", "所属场景": "S001-KT", "数量": 2}}},
		{"素材.物品": map[string]any{"食材": map[string]any{"所属场景": "S001-KT", "数量": 2}}},
	}
	for index, data := range views {
		items, err := readItems(NewSnapshot(data))
		if err != nil {
			t.Fatalf("view %d: readItems: %v", index, err)
		}
		if len(items) != 1 || items[0].Name != "食材" || items[0].Scene != "S001-KT" || items[0].Quantity != 2 {
			t.Fatalf("view %d: items = %+v", index, items)
		}
	}
}

// TestSnapshotAdapterErrors proves unreadable paths fail loudly instead of
// decoding into an empty view.
func TestSnapshotAdapterErrors(t *testing.T) {
	if _, err := NewSnapshot(map[string]any{"代谢": 1}).Get("代谢.胃内容"); err == nil {
		t.Fatal("Get through a scalar object succeeded")
	}
	if _, err := NewSnapshot(nil).Get("代谢.胃内容"); err == nil {
		t.Fatal("Get from nil data succeeded")
	}
	if _, err := NewSnapshot(map[string]any{}).Get(""); err == nil {
		t.Fatal("Get with an empty path succeeded")
	}
	if _, err := NewSnapshot(map[string]any{}).Get("代谢.胃内容"); err == nil {
		t.Fatal("Get of an unknown path succeeded")
	}
}

// TestSnapshotImplementsCondition keeps the K2 adapter contract checked at
// compile time in addition to snapshot.go's assertion.
func TestSnapshotImplementsCondition(t *testing.T) {
	var _ condition.Snapshot = NewSnapshot(map[string]any{})
}
