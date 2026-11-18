// items.go resolves the read-only 素材.物品 view and plans the produce rows.
// Consume is an interaction intent only, so this file never mutates a consumed
// material.
package settle

import (
	"fmt"
	"sort"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Item-record keys shared with the material kernel model.
const (
	// KEY_ITEM_NAME is the material kernel's 名称 field.
	KEY_ITEM_NAME = "名称"
	// KEY_ITEM_SCENE is the material kernel's 所属场景 field.
	KEY_ITEM_SCENE = "所属场景"
	// KEY_ITEM_QUANTITY is the material kernel's 数量 field.
	KEY_ITEM_QUANTITY = "数量"
)

// Item is one decoded 素材.物品 row. Scene is the material kernel's 所属场景 and
// Quantity its 数量; both are required for C5 resolution.
type Item struct {
	Name     string
	Scene    string
	Quantity int
}

// planProduce builds one item creation row per produced item name. Duplicate
// names are aggregated; the template shape of the first occurrence is kept so a
// store can create the material entity with its type and description.
func planProduce(flow spec.FlowSpec) ([]Change, []error) {
	if len(flow.Produce) == 0 {
		return nil, nil
	}
	order := make([]string, 0, len(flow.Produce))
	counts := make(map[string]int, len(flow.Produce))
	templates := make(map[string]spec.ItemSpec, len(flow.Produce))
	var errs []error
	for index, item := range flow.Produce {
		if item.Name == "" {
			errs = append(errs, fmt.Errorf("produce[%d]: empty item name", index))
			continue
		}
		if _, seen := counts[item.Name]; !seen {
			order = append(order, item.Name)
			templates[item.Name] = item
		}
		counts[item.Name]++
	}
	changes := make([]Change, 0, len(order))
	for _, name := range order {
		template := templates[name]
		changes = append(changes, Change{
			Op:     OP_PRODUCE,
			Entity: name,
			Scene:  flow.Scene,
			Path:   PATH_ITEM_QUANTITY,
			Kind:   spec.EFFECT_KIND_ADD,
			Value:  counts[name],
			Item:   &template,
		})
	}
	return changes, errs
}

// readItems decodes condition.PATH_ITEMS. The accepted shapes are a list of
// item records or a map from item name to record; any other shape is an error
// rather than a silent empty view.
func readItems(snap condition.Snapshot) ([]Item, error) {
	if snap == nil {
		return nil, fmt.Errorf("%s: nil snapshot", condition.PATH_ITEMS)
	}
	raw, err := snap.Get(condition.PATH_ITEMS)
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
		return nil, fmt.Errorf("%s: want item list or map, got %T", condition.PATH_ITEMS, raw)
	}
}

// decodeItemList decodes a list-ordered item view.
func decodeItemList(entries []any) ([]Item, error) {
	items := make([]Item, 0, len(entries))
	for index, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: want item record, got %T", condition.PATH_ITEMS, index, entry)
		}
		item, err := decodeItem(record, "")
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", condition.PATH_ITEMS, index, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// decodeItemMap decodes a name-keyed item view in sorted key order so malformed
// input reports the same error on every run.
func decodeItemMap(view map[string]any) ([]Item, error) {
	names := make([]string, 0, len(view))
	for name := range view {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]Item, 0, len(names))
	for _, name := range names {
		record, ok := view[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%q]: want item record, got %T", condition.PATH_ITEMS, name, view[name])
		}
		item, err := decodeItem(record, name)
		if err != nil {
			return nil, fmt.Errorf("%s[%q]: %w", condition.PATH_ITEMS, name, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// decodeItem decodes one item record. The name falls back to the map key when
// the record omits it; scene defaults to empty and quantity to the kernel's
// per-item default of 1.
func decodeItem(record map[string]any, fallbackName string) (Item, error) {
	name, err := itemName(record, fallbackName)
	if err != nil {
		return Item{}, err
	}
	scene, err := itemScene(record)
	if err != nil {
		return Item{}, err
	}
	quantity, err := itemQuantity(record)
	if err != nil {
		return Item{}, err
	}
	return Item{Name: name, Scene: scene, Quantity: quantity}, nil
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
	quantity, err := toInteger(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", KEY_ITEM_QUANTITY, err)
	}
	if quantity < 0 {
		return 0, fmt.Errorf("%s: negative quantity %d", KEY_ITEM_QUANTITY, quantity)
	}
	return quantity, nil
}

// findItemRow resolves one item by (name, scene). An empty scene matches the
// first row of that name; otherwise only the template scene instance matches,
// which is the C5 same-name cross-scene rule.
func findItemRow(items []Item, name, scene string) (Item, bool) {
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if scene == "" || item.Scene == scene {
			return item, true
		}
	}
	return Item{}, false
}
