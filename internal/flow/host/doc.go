// Package host wires the frozen P4 flow seams (K1–K4) to the real entity store.
//
// The package is the integration layer only: it owns no flow semantics. Snapshot
// adapts one *entity.Entity to condition.Snapshot, ItemsView projects the
// material entities into the 素材.物品 view, EntityStore implements settle.Store
// and task.Store, and Ledger records the single settlement and task advance of
// every (flowID, entity) pair.
//
// Write model: EntityStore applies one settlement write set all-or-nothing. It
// deep-copies every touched entity first, applies each row to the copy, and only
// then writes the real entities; a write the entity rejects rolls the already
// written rows back to their pre-batch values, so a refused settlement never
// leaves a partial write. Settlement writes stay in memory: the caller decides
// when to persist by calling Flush, because the engine owns the tick.
//
// Concurrency: callers must serialize Apply, task.Store.Set and Flush under the
// engine's tick write gate. entity.Entity locks one field write at a time, so
// cross-entity atomicity is a host responsibility, not an entity guarantee.
//
// Contract gap K4-TXN: entity.Entity exposes Snapshot/Set but no transactional
// replace, so atomicity is emulated rather than provided by the store. Every row
// is validated on a deep copy before the first write; a write the entity rejects
// (a readonly annotation) rolls the batch back, and an incomplete rollback is
// reported instead of hidden.
package host
