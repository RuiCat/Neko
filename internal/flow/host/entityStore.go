// entityStore.go implements the two write seams over one real entity.Store:
// settle.Store.Apply applies a whole settlement batch all-or-nothing, and
// task.Store reads and writes one field. Persistence is explicit: both seams only
// mutate in-memory entity state, and Flush writes the store to disk.
package host

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/task"
)

// EntityStore adapts one entity store to the K4 settlement store and the K4 task
// store. It keeps no copy of the data: reads and writes go to the entities the
// store owns.
type EntityStore struct {
	store *entity.Store
}

// NewEntityStore wraps a loaded entity store.
func NewEntityStore(store *entity.Store) *EntityStore {
	return &EntityStore{store: store}
}

// Get implements task.Store. An absent entity or an unknown path reports false,
// which is how task advancement reads a task that never started as 未激活.
func (s *EntityStore) Get(entityID, path string) (any, bool) {
	if s.store == nil {
		return nil, false
	}
	ent := s.store.Get(entityID)
	if ent == nil {
		return nil, false
	}
	value, err := ent.Get(path)
	if err != nil {
		return nil, false
	}
	return value, true
}

// Set implements task.Store. The write lands in the entity's memory; call Flush
// to persist it. A missing entity is an error because silently dropping a task
// write would let an advance report success without a row.
func (s *EntityStore) Set(entityID, path string, value any) error {
	if s.store == nil {
		return fmt.Errorf("set %s on %q: no entity store", path, entityID)
	}
	ent := s.store.Get(entityID)
	if ent == nil {
		return fmt.Errorf("set %s: entity %q not found", path, entityID)
	}
	return ent.Set(path, value)
}

// Flush persists all entities through entity.Store.Save (temp file + rename per
// file). The caller decides when to flush, because the engine persists once per
// tick under its own write gate.
func (s *EntityStore) Flush() error {
	if s.store == nil {
		return fmt.Errorf("flush: no entity store")
	}
	return s.store.Save()
}

// Apply implements settle.Store: one settlement write set is validated on deep
// copies and then applied to the real entities, so a refused row leaves no
// partial write behind. The batch is not persisted here; call Flush afterwards.
func (s *EntityStore) Apply(flowID string, changes []settle.Change) error {
	if s.store == nil {
		return fmt.Errorf("flow %q: no entity store", flowID)
	}
	plan, err := newApplyPlan(s.store, changes)
	if err != nil {
		return fmt.Errorf("flow %q: %w", flowID, err)
	}
	if err := plan.commit(); err != nil {
		return fmt.Errorf("flow %q: %w", flowID, err)
	}
	return nil
}

// EntityStore is the settlement target.
var _ settle.Store = (*EntityStore)(nil)

// EntityStore is the task advancement target.
var _ task.Store = (*EntityStore)(nil)
