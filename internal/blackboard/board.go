// board.go is the minimal per-turn coordination surface (C2): one current
// event, one hard-state snapshot and one constraint set. It keeps no history
// and is the only thing experts read besides their own private context.
package blackboard

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/RuiCat/Neko/internal/hardstate"
)

// MAX_SERIALIZED_BYTES bounds the whole board; exceeding it means the board is
// being used as history storage, which the architecture forbids.
const MAX_SERIALIZED_BYTES = 8 * 1024

// Event describes what happened this turn, structurally and without
// interpretation.
type Event struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Summary string         `json:"summary"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Constraints are the scene limits of this turn.
type Constraints struct {
	Location          string   `json:"location,omitempty"`
	Time              string   `json:"time,omitempty"`
	Present           []string `json:"present,omitempty"`
	KnowledgeBoundary []string `json:"knowledge_boundary,omitempty"`
}

// Reader is the read-only view handed to experts; it exposes no setters.
type Reader interface {
	Event() Event
	Snapshot() hardstate.Snapshot
	Constraints() Constraints
}

// Board holds exactly one turn's coordination fields. It is safe for
// concurrent use; readers must treat returned maps as read-only.
type Board struct {
	mu          sync.RWMutex
	event       Event
	snapshot    hardstate.Snapshot
	constraints Constraints
}

// New returns an empty board.
func New() *Board {
	return &Board{}
}

// SetEvent replaces the current event. The whole board must stay under the
// serialized size cap.
func (b *Board) SetEvent(event Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	previous := b.event
	b.event = event
	return b.checkSizeLocked(&previous)
}

// SetSnapshot replaces the current hard-state snapshot.
func (b *Board) SetSnapshot(snapshot hardstate.Snapshot) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	previous := b.snapshot
	b.snapshot = snapshot
	return b.checkSizeLocked(&previous)
}

// SetConstraints replaces the scene constraints.
func (b *Board) SetConstraints(constraints Constraints) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	previous := b.constraints
	b.constraints = constraints
	return b.checkSizeLocked(&previous)
}

// Event returns the current event.
func (b *Board) Event() Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.event
}

// Snapshot returns the current snapshot (treat the maps as read-only).
func (b *Board) Snapshot() hardstate.Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snapshot
}

// Constraints returns the current constraints.
func (b *Board) Constraints() Constraints {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.constraints
}

// checkSizeLocked validates the current board and rolls one field back when it
// is too large. Callers hold b.mu.
func (b *Board) checkSizeLocked(previous any) error {
	raw, err := json.Marshal(struct {
		Event       Event              `json:"event"`
		Snapshot    hardstate.Snapshot `json:"snapshot"`
		Constraints Constraints        `json:"constraints"`
	}{b.event, b.snapshot, b.constraints})
	if err != nil {
		return fmt.Errorf("blackboard: 序列化失败: %w", err)
	}
	if len(raw) <= MAX_SERIALIZED_BYTES {
		return nil
	}
	switch value := previous.(type) {
	case *Event:
		b.event = *value
	case *hardstate.Snapshot:
		b.snapshot = *value
	case *Constraints:
		b.constraints = *value
	}
	return fmt.Errorf("blackboard: 序列化 %d 字节超过上限 %d", len(raw), MAX_SERIALIZED_BYTES)
}
