// memoryLedger.go provides the process-local Ledger used by tests and hosts
// that settle within one engine lifetime. Persisting the ledger is the host's
// concern; settlement only needs lookup and record.
package settle

import "sync"

// settlementKey identifies the one allowed settlement of a flow on an entity.
type settlementKey struct {
	flowID string
	entity string
}

// MemoryLedger records settled (flowID, entity) pairs in memory. The zero value
// is usable and safe for concurrent lookups; a host that spans process
// restarts must supply a durable Ledger instead.
type MemoryLedger struct {
	mu        sync.Mutex
	completed map[settlementKey]Completion
}

// NewMemoryLedger returns an empty ledger.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{completed: make(map[settlementKey]Completion)}
}

// Lookup returns the recorded completion for (flowID, entity), if any.
func (l *MemoryLedger) Lookup(flowID, entity string) (Completion, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	completion, ok := l.completed[settlementKey{flowID: flowID, entity: entity}]
	return completion, ok
}

// Record stores one completion, overwriting only the same (flowID, entity).
func (l *MemoryLedger) Record(completion Completion) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.completed == nil {
		l.completed = make(map[settlementKey]Completion)
	}
	l.completed[settlementKey{flowID: completion.FlowID, entity: completion.Entity}] = completion
}
