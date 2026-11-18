// fakeStore.go is the in-memory entity view the harness settles into. It
// implements both write seams (settlement and task advancement), keeps the
// snapshot and the store on one map so a later read observes earlier writes, and
// simulates the trade tick for spend requests because settlement only files the
// request (C5). Apply is atomic by cloning the entity before any row is applied.
package verify

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/task"
)

// PATH_HUNGER is the status-kernel hunger flag the 吃饭 effect clears. K1 maps
// it to the same-name source field, so the harness asserts it after settlement.
const PATH_HUNGER = "标记.饥饿"

// fakeStore is one entity view plus a per-path write counter. The counters let a
// test prove that a reward row was written once, not twice.
type fakeStore struct {
	data   map[string]any
	writes map[string]int
}

// newFakeStore wraps one entity map. The map is owned by the store from here on.
func newFakeStore(data map[string]any) *fakeStore {
	if data == nil {
		data = map[string]any{}
	}
	return &fakeStore{data: data, writes: map[string]int{}}
}

// Apply implements settle.Store. It validates and applies the whole change set
// on a clone and swaps the clone in only when every row succeeded, which mirrors
// the atomic Store contract settlement relies on.
func (s *fakeStore) Apply(flowID string, changes []settle.Change) error {
	next := cloneEntity(s.data)
	for index, change := range changes {
		if err := applyChange(next, change); err != nil {
			return fmt.Errorf("flow %q change[%d]: %w", flowID, index, err)
		}
	}
	s.data = next
	return nil
}

// Get implements task.Store for the single-entity harness. The entity name is
// carried by the caller only; the harness owns one entity view.
func (s *fakeStore) Get(entity, path string) (any, bool) {
	value, err := s.value(path)
	if err != nil {
		return nil, false
	}
	return value, true
}

// Set implements task.Store and counts one write for path.
func (s *fakeStore) Set(entity, path string, value any) error {
	if s.data == nil {
		return fmt.Errorf("set %s: no entity data", path)
	}
	s.data[path] = value
	s.writes[path]++
	return nil
}

// setScene moves the actor to scene, modelling the world step between flows.
func (s *fakeStore) setScene(scene string) {
	s.data[condition.PATH_SCENE] = scene
}

// snapshot exposes the current entity view to condition.Judge and settlement.
func (s *fakeStore) snapshot() settle.Snapshot {
	return settle.NewSnapshot(s.data)
}

// value resolves one dotted entity path through the settlement adapter.
func (s *fakeStore) value(path string) (any, error) {
	return s.snapshot().Get(path)
}

// intAt reads one integer row and reports whether it exists as an integer.
func (s *fakeStore) intAt(path string) (int, bool) {
	value, err := s.value(path)
	if err != nil {
		return 0, false
	}
	return asInt(value)
}

// writeCount reports how often task.Store.Set wrote path.
func (s *fakeStore) writeCount(path string) int {
	return s.writes[path]
}

// itemQuantity sums the quantity of one item name in one scene.
func (s *fakeStore) itemQuantity(name, scene string) int {
	rows, err := itemRows(s.data)
	if err != nil {
		return -1
	}
	total := 0
	for _, row := range rows {
		if row[condition.KEY_ITEM_NAME] != name {
			continue
		}
		if scene != "" && row[condition.KEY_ITEM_SCENE] != scene {
			continue
		}
		quantity, ok := asInt(row[condition.KEY_ITEM_QUANTITY])
		if !ok {
			return -1
		}
		total += quantity
	}
	return total
}

// applyChange dispatches one settlement row by its operation.
func applyChange(data map[string]any, change settle.Change) error {
	switch change.Op {
	case settle.OP_SPEND:
		return applySpend(data, change)
	case settle.OP_CONSUME, settle.OP_PRODUCE:
		return applyItem(data, change)
	case settle.OP_EFFECT:
		return applyEffect(data, change)
	default:
		return fmt.Errorf("unknown change op %q", change.Op)
	}
}

// applySpend records the trade request rows and simulates the trade tick: the
// 成交额 row debits the balance, which settlement itself must not assign (C5).
func applySpend(data map[string]any, change settle.Change) error {
	if change.Path == settle.PATH_TRADE_AMOUNT {
		amount, ok := asInt(change.Value)
		if !ok || amount < 0 {
			return fmt.Errorf("%s: want a non-negative integer, got %v", change.Path, change.Value)
		}
		balance, ok := asInt(data[condition.PATH_MONEY])
		if !ok {
			return fmt.Errorf("%s: no integer balance to debit", condition.PATH_MONEY)
		}
		if balance < amount {
			return fmt.Errorf("%s: balance %d cannot pay %d", condition.PATH_MONEY, balance, amount)
		}
		data[condition.PATH_MONEY] = balance - amount
	}
	data[change.Path] = change.Value
	return nil
}

// applyItem applies one quantity delta to the material row identified by
// (name, scene), creating the row for a non-negative produce delta.
func applyItem(data map[string]any, change settle.Change) error {
	delta, ok := asInt(change.Value)
	if !ok {
		return fmt.Errorf("item %q: quantity %v is not an integer", change.Entity, change.Value)
	}
	rows, err := itemRows(data)
	if err != nil {
		return err
	}
	index := findItemRow(rows, change.Entity, change.Scene)
	if index < 0 {
		if delta < 0 {
			return fmt.Errorf("item %q in scene %q: no row to remove %d", change.Entity, change.Scene, -delta)
		}
		rows = append(rows, newItemRow(change, delta))
	} else {
		current, ok := asInt(rows[index][condition.KEY_ITEM_QUANTITY])
		if !ok {
			return fmt.Errorf("item %q: quantity is not an integer", change.Entity)
		}
		if current+delta < 0 {
			return fmt.Errorf("item %q in scene %q: %d - %d leaves a negative quantity", change.Entity, change.Scene, current, -delta)
		}
		rows[index][condition.KEY_ITEM_QUANTITY] = current + delta
	}
	entries := make([]any, len(rows))
	for entryIndex, row := range rows {
		entries[entryIndex] = row
	}
	data[condition.PATH_ITEMS] = entries
	return nil
}

// applyEffect writes one source-layer field according to its K1 kind. A set
// replaces the value, an add is relative to the current row (absent means zero).
func applyEffect(data map[string]any, change settle.Change) error {
	switch change.Kind {
	case spec.EFFECT_KIND_SET, spec.EFFECT_KIND_ADD:
		value, ok := asInt(change.Value)
		if !ok {
			return fmt.Errorf("%s: %s wants an integer, got %v", change.Path, change.Kind, change.Value)
		}
		if change.Kind == spec.EFFECT_KIND_ADD {
			current, _ := asInt(data[change.Path])
			value = current + value
		}
		data[change.Path] = value
	case spec.EFFECT_KIND_TEXT:
		text, ok := change.Value.(string)
		if !ok {
			return fmt.Errorf("%s: text wants a string, got %v", change.Path, change.Value)
		}
		data[change.Path] = text
	case spec.EFFECT_KIND_BOOL:
		flag, ok := change.Value.(bool)
		if !ok {
			return fmt.Errorf("%s: bool wants a bool, got %v", change.Path, change.Value)
		}
		data[change.Path] = flag
	default:
		return fmt.Errorf("%s: unknown effect kind %q", change.Path, change.Kind)
	}
	return nil
}

// itemRows decodes the material view the harness builds (a list of records).
func itemRows(data map[string]any) ([]map[string]any, error) {
	raw, ok := data[condition.PATH_ITEMS]
	if !ok || raw == nil {
		return nil, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: harness stores a list, got %T", condition.PATH_ITEMS, raw)
	}
	rows := make([]map[string]any, 0, len(entries))
	for index, entry := range entries {
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: want a record, got %T", condition.PATH_ITEMS, index, entry)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// findItemRow returns the index of the (name, scene) row, or -1.
func findItemRow(rows []map[string]any, name, scene string) int {
	for index, row := range rows {
		if row[condition.KEY_ITEM_NAME] != name {
			continue
		}
		if scene == "" || row[condition.KEY_ITEM_SCENE] == scene {
			return index
		}
	}
	return -1
}

// newItemRow builds the material record a produce row creates.
func newItemRow(change settle.Change, quantity int) map[string]any {
	row := map[string]any{
		condition.KEY_ITEM_NAME:     change.Entity,
		condition.KEY_ITEM_SCENE:    change.Scene,
		condition.KEY_ITEM_QUANTITY: quantity,
	}
	if change.Item != nil {
		row["类型"] = change.Item.Type
		row["描述"] = change.Item.Description
	}
	return row
}

// itemRow builds one fixture material record.
func itemRow(name, scene string, quantity int) map[string]any {
	return map[string]any{
		condition.KEY_ITEM_NAME:     name,
		condition.KEY_ITEM_SCENE:    scene,
		condition.KEY_ITEM_QUANTITY: quantity,
	}
}

// cloneEntity copies one entity view deeply enough for Apply to stay atomic:
// the material list and its record maps are cloned, scalar rows are shared.
func cloneEntity(data map[string]any) map[string]any {
	copied := make(map[string]any, len(data))
	for key, value := range data {
		entries, ok := value.([]any)
		if !ok {
			copied[key] = value
			continue
		}
		clonedEntries := make([]any, len(entries))
		for index, entry := range entries {
			row, ok := entry.(map[string]any)
			if !ok {
				clonedEntries[index] = entry
				continue
			}
			clonedRow := make(map[string]any, len(row))
			for rowKey, rowValue := range row {
				clonedRow[rowKey] = rowValue
			}
			clonedEntries[index] = clonedRow
		}
		copied[key] = clonedEntries
	}
	return copied
}

// asInt coerces the numeric shapes a store or JSON artifact can hand back.
// Fractional and non-numeric values are rejected instead of truncated.
func asInt(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float32:
		return floatAsInt(float64(value))
	case float64:
		return floatAsInt(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

// floatAsInt accepts only finite integral floats.
func floatAsInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

// The harness relies on both seams being satisfied by one fake.
var (
	_ settle.Store = (*fakeStore)(nil)
	_ task.Store   = (*fakeStore)(nil)
)
