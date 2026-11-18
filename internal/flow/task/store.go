// store.go declares the two seams task advancement needs at run time: the entity
// store and the single-advance ledger, plus the in-process ledger used by tests
// and the host. Task advancement owns no I/O, so every refusal stays visible to
// the caller as an error or a Replay transition.
package task

import "sync"

// Store reads and writes one entity field. Get reports false when the field is
// absent, which is how a task that never started reads as 未激活. Set writes one
// field and is only ever called with the paths this package declares in paths.go.
type Store interface {
	Get(entity, path string) (any, bool)
	Set(entity, path string, value any) error
}

// Ledger records the Transition produced for one (flowID, entity) pair. A
// replayed completion looks the record up and returns it without touching the
// store, which is what makes the reward single-settlement (C4).
type Ledger interface {
	Lookup(flowID, entity string) (Transition, bool)
	Record(transition Transition)
}

// ledgerKey identifies one advanced completion. K4 freezes (FlowID, Entity) as
// the settlement identity, so the ledger keys on the same pair.
type ledgerKey struct {
	flowID string
	entity string
}

// MemoryLedger is the in-process Ledger. The host advances under the engine's
// tick-serialized write gate, so Lookup followed by Record cannot interleave for
// one key; the mutex keeps the map race-free for concurrent readers.
type MemoryLedger struct {
	mu      sync.Mutex
	records map[ledgerKey]Transition
}

// NewMemoryLedger returns an empty in-process ledger.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{records: map[ledgerKey]Transition{}}
}

// Lookup returns the transition recorded for one completion identity.
func (l *MemoryLedger) Lookup(flowID, entity string) (Transition, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recorded, ok := l.records[ledgerKey{flowID: flowID, entity: entity}]
	return recorded, ok
}

// Record stores one transition under the identity carried by the transition.
func (l *MemoryLedger) Record(transition Transition) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records[ledgerKey{flowID: transition.FlowID, entity: transition.Entity}] = transition
}
