// items.go decodes the read-only item view and resolves one required item by
// (name, scene, quantity). Name uniqueness per scene comes from the material
// kernel; same-name items in other scenes are deliberately ignored (C5).
package condition

import (
	"fmt"
	"sort"
)

// Item-record keys shared with the material kernel model.
const (
	KEY_ITEM_NAME     = "名称"
	KEY_ITEM_SCENE    = "所属场景"
	KEY_ITEM_QUANTITY = "数量"
)

// itemRecord is one decoded item-view row.
type itemRecord struct {
	name     string
	scene    string
	quantity int
}

// readItems decodes PATH_ITEMS. Accepted shapes are a list of item records or a
// map from item name to record; any other shape is an eval_error rather than a
// silent empty view.
func readItems(snap Snapshot) ([]itemRecord, error) {
	raw, err := snap.Get(PATH_ITEMS)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	switch view := raw.(type) {
	case []any:
		return decodeItemList(view)
	case []map[string]any:
		entries := make([]any, len(view))
		for index, entry := range view {
			entries[index] = entry
		}
		return decodeItemList(entries)
	case map[string]any:
		return decodeItemMap(view)
	default:
		return nil, fmt.Errorf("%s: want item list or map, got %T", PATH_ITEMS, raw)
	}
}

// decodeItemList decodes a list-ordered item view.
func decodeItemList(entries []any) ([]itemRecord, error) {
	items := make([]itemRecord, 0, len(entries))
	for index, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: want item record, got %T", PATH_ITEMS, index, entry)
		}
		item, err := decodeItem(record, "")
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", PATH_ITEMS, index, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// decodeItemMap decodes a name-keyed item view in sorted key order so malformed
// input reports the same error on every run.
func decodeItemMap(view map[string]any) ([]itemRecord, error) {
	names := make([]string, 0, len(view))
	for name := range view {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]itemRecord, 0, len(names))
	for _, name := range names {
		record, ok := view[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%q]: want item record, got %T", PATH_ITEMS, name, view[name])
		}
		item, err := decodeItem(record, name)
		if err != nil {
			return nil, fmt.Errorf("%s[%q]: %w", PATH_ITEMS, name, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// decodeItem decodes one item record. The name falls back to the map key when
// the record omits it; scene defaults to empty and quantity to the kernel's
// per-item default of 1.
func decodeItem(record map[string]any, fallbackName string) (itemRecord, error) {
	name, err := itemName(record, fallbackName)
	if err != nil {
		return itemRecord{}, err
	}
	scene, err := itemScene(record)
	if err != nil {
		return itemRecord{}, err
	}
	quantity, err := itemQuantity(record)
	if err != nil {
		return itemRecord{}, err
	}
	return itemRecord{name: name, scene: scene, quantity: quantity}, nil
}

// itemName resolves the item name, preferring the record field over the key.
func itemName(record map[string]any, fallbackName string) (string, error) {
	raw, ok := record[KEY_ITEM_NAME]
	if !ok || raw == nil {
		if fallbackName == "" {
			return "", fmt.Errorf("missing %s", KEY_ITEM_NAME)
		}
		return fallbackName, nil
	}
	name, ok := raw.(string)
	if !ok || name == "" {
		return "", fmt.Errorf("%s: want non-empty string, got %T", KEY_ITEM_NAME, raw)
	}
	return name, nil
}

// itemScene resolves 所属场景; an absent scene stays empty (unconstrained).
func itemScene(record map[string]any) (string, error) {
	raw, ok := record[KEY_ITEM_SCENE]
	if !ok || raw == nil {
		return "", nil
	}
	scene, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: want string, got %T", KEY_ITEM_SCENE, raw)
	}
	return scene, nil
}

// itemQuantity resolves 数量; an absent quantity defaults to 1 and a negative
// quantity is rejected as malformed input.
func itemQuantity(record map[string]any) (int, error) {
	raw, ok := record[KEY_ITEM_QUANTITY]
	if !ok || raw == nil {
		return 1, nil
	}
	quantity, ok := toInt(raw)
	if !ok {
		return 0, fmt.Errorf("%s: want integer, got %T", KEY_ITEM_QUANTITY, raw)
	}
	if quantity < 0 {
		return 0, fmt.Errorf("%s: negative quantity %d", KEY_ITEM_QUANTITY, quantity)
	}
	return quantity, nil
}

// countItemQuantity sums the quantity of items matching name and scene. An
// empty scene counts every scene (K1 scene "" means unconstrained).
func countItemQuantity(items []itemRecord, name, scene string) int {
	total := 0
	for _, item := range items {
		if item.name != name {
			continue
		}
		if scene != "" && item.scene != scene {
			continue
		}
		total += item.quantity
	}
	return total
}
