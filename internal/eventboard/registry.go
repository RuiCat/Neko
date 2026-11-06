// registry.go owns the process-wide set of per-entity boards exactly once:
// load, observe baselines, fold a tick and hand read-only copies to readers.
package eventboard

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
)

// Registry is the concurrent owner of every entity's board. The tick writer
// mutates it under tickMu, so the lock here only separates those writes from
// control-plane readers; a mutation and its save happen in one critical
// section, so a reader never observes a half-applied tick.
type Registry struct {
	mu     sync.RWMutex
	path   string
	boards map[string]*Board
	// now is the clock; tests pin it, production uses time.Now.
	now func() time.Time
}

// New returns an empty registry that persists to path.
func New(path string) *Registry {
	return &Registry{path: path, boards: map[string]*Board{}, now: time.Now}
}

// Load reads the board file. A missing file leaves the registry empty; a
// malformed one returns the error and keeps the registry empty so the caller
// can log it and keep ticking.
func (r *Registry) Load() error {
	boards, err := loadBoards(r.path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.boards = boards
	return nil
}

// Observe records entity ids whose board must exist without a change: the
// first observation only fixes the baseline, so it creates no entries and does
// not bump a version. Boards loaded from disk or changed earlier are kept.
func (r *Registry) Observe(entityIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entityID := range entityIDs {
		if entityID == "" {
			continue
		}
		if _, exists := r.boards[entityID]; exists {
			continue
		}
		r.boards[entityID] = newBoard(entityID)
	}
}

// Apply folds one finished tick into the boards and persists the result. Board
// entities never seen before start from an empty baseline and then receive this
// tick's changes. The caller keeps the tick's fate independent: an Apply error
// is logged by the caller and never fails the tick (K2).
func (r *Registry) Apply(result engine.TickResult) error {
	groups := aggregateTransitions(result.Transitions)
	if len(groups) == 0 {
		return nil
	}
	at := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for _, group := range groups {
		board, exists := r.boards[group.Entity]
		if !exists {
			board = newBoard(group.Entity)
			r.boards[group.Entity] = board
		}
		if board.apply(group.Changes, result.TickID, at) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := saveBoards(r.path, r.boards); err != nil {
		return fmt.Errorf("保存事件黑板失败: %w", err)
	}
	return nil
}

// Board returns a copy of one entity's board, or nil when the entity has none.
// The copy keeps readers independent from the next tick's in-place update.
func (r *Registry) Board(entityID string) *Board {
	r.mu.RLock()
	defer r.mu.RUnlock()
	board, exists := r.boards[entityID]
	if !exists {
		return nil
	}
	return board.clone()
}

// Boards returns every board sorted by entity.
func (r *Registry) Boards() []*Board {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entities := make([]string, 0, len(r.boards))
	for entity := range r.boards {
		entities = append(entities, entity)
	}
	sort.Strings(entities)
	boards := make([]*Board, 0, len(entities))
	for _, entity := range entities {
		boards = append(boards, r.boards[entity].clone())
	}
	return boards
}
