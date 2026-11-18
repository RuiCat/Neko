// snapshot.go adapts one real entity to the read-only condition.Snapshot the
// judge and settlement consume. It exposes no write method and never mutates the
// entity; 素材.物品 falls back to the live material view when the entity carries
// no native field for it.
package host

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/flow/condition"
)

// Snapshot is the read-only condition.Snapshot over one real entity. Paths other
// than 素材.物品 resolve through entity.Get, so template fallbacks read exactly the
// values the engine sees.
type Snapshot struct {
	entity *entity.Entity
	items  ItemsView
}

// NewSnapshot binds one entity to the live material view. The entity is read,
// never written; callers must not mutate it while a judgment or settlement is in
// flight.
func NewSnapshot(ent *entity.Entity, items ItemsView) Snapshot {
	return Snapshot{entity: ent, items: items}
}

// Snapshot looks one entity up and attaches the live material view. The
// returned view is bound to the entity, not to a copy: a later settlement write
// is visible through it.
func (s *EntityStore) Snapshot(entityID string) (Snapshot, error) {
	if s.store == nil {
		return Snapshot{}, fmt.Errorf("snapshot %q: no entity store", entityID)
	}
	ent := s.store.Get(entityID)
	if ent == nil {
		return Snapshot{}, fmt.Errorf("snapshot %q: entity not found", entityID)
	}
	return NewSnapshot(ent, NewItemsView(s.store)), nil
}

// Get resolves one snapshot path. A native 素材.物品 field wins because it is real
// data; otherwise the live material view is projected, which is what lets a
// produced item be judged on the next decision.
func (s Snapshot) Get(path string) (any, error) {
	if s.entity == nil {
		return nil, fmt.Errorf("%s: no entity", path)
	}
	if path != condition.PATH_ITEMS {
		return s.entity.Get(path)
	}
	if value, err := s.entity.Get(path); err == nil {
		return value, nil
	}
	return s.items.View()
}

// Snapshot implements the K2 read-only view.
var _ condition.Snapshot = Snapshot{}
