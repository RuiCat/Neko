// itemsView.go projects the material-kernel entities into the read-only
// 素材.物品 view K2 and K4 consume. One material entity is one row; a row carries
// 名称/所属场景/数量 only, so judgment and settlement resolve an item by the same
// (name, scene, quantity) triad (C5).
package host

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/flow/condition"
)

// MATERIAL_TYPE is the material-kernel template every item instance inherits.
const MATERIAL_TYPE = "素材"

// ItemsView is the read-only projection of the material entities of one store.
// It reads the store on every call, so a settlement that creates a material is
// visible to the next judgment without rebuilding the view.
type ItemsView struct {
	store *entity.Store
}

// NewItemsView binds the view to one entity store. The view never writes.
func NewItemsView(store *entity.Store) ItemsView {
	return ItemsView{store: store}
}

// View returns one record per material entity in entity-id order. A nil store
// is an error rather than an empty view: a missing view must not read as "no
// items", which would turn a wiring bug into a wrong judgment.
func (v ItemsView) View() ([]any, error) {
	if v.store == nil {
		return nil, fmt.Errorf("%s: no entity store", condition.PATH_ITEMS)
	}
	entities := v.store.ListAll()
	rows := make([]any, 0, len(entities))
	for _, ent := range entities {
		if !entity.IsSubtypeOf(ent.Type, MATERIAL_TYPE) {
			continue
		}
		row, err := materialRow(ent)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", condition.PATH_ITEMS, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// materialRow renders one material entity as an item record. The name is
// required; an absent scene stays empty (unconstrained) and an absent quantity
// takes the kernel default of 1, matching the K2/K4 decoders.
func materialRow(ent *entity.Entity) (map[string]any, error) {
	name, err := materialText(ent, condition.KEY_ITEM_NAME)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("%s: entity %q has an empty %s", ent.ID, ent.ID, condition.KEY_ITEM_NAME)
	}
	scene, err := materialText(ent, condition.KEY_ITEM_SCENE)
	if err != nil {
		return nil, err
	}
	quantity, err := materialQuantity(ent)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		condition.KEY_ITEM_NAME:     name,
		condition.KEY_ITEM_SCENE:    scene,
		condition.KEY_ITEM_QUANTITY: quantity,
	}, nil
}

// materialText reads one string field. An absent field reads as empty so a
// template without optional metadata still projects.
func materialText(ent *entity.Entity, path string) (string, error) {
	value := ent.GetOrNone(path)
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("entity %q: %s is not a string", ent.ID, path)
	}
	return text, nil
}

// materialQuantity reads 数量 and coerces it to int (JSON reloads yield
// float64). An absent quantity takes the material kernel's default of 1.
func materialQuantity(ent *entity.Entity) (int, error) {
	raw := ent.GetOrNone(condition.KEY_ITEM_QUANTITY)
	if raw == nil {
		return 1, nil
	}
	quantity, err := intValue(raw, fmt.Sprintf("entity %q %s", ent.ID, condition.KEY_ITEM_QUANTITY))
	if err != nil {
		return 0, err
	}
	if quantity < 0 {
		return 0, fmt.Errorf("entity %q: negative %s %d", ent.ID, condition.KEY_ITEM_QUANTITY, quantity)
	}
	return quantity, nil
}
