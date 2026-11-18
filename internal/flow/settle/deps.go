// deps.go declares the three seams settlement needs at run time: the read-only
// entity snapshot (K2's condition.Snapshot), an atomic store and the single-
// settlement ledger. Settlement owns no I/O itself, so every failure stays
// visible to the caller as a Report error.
package settle

import "github.com/RuiCat/Neko/internal/flow/condition"

// Store applies one fully validated settlement write set. Implementations MUST
// be all-or-nothing: when Apply returns an error no row may be persisted, so a
// refused settlement can never leave partial writes behind (K4). Actor rows
// carry the entity name; consume/produce rows carry the item name and scene,
// and produce rows also carry Item so a store can create the material entity.
type Store interface {
	Apply(flowID string, changes []Change) error
}

// Ledger records the single settlement per (flowID, entity) and returns it on
// replay. The host calls settlement under the engine's tick-serialized write
// gate, so Lookup followed by Record cannot interleave for one key; the
// in-memory implementation still locks to keep its map race-free.
type Ledger interface {
	Lookup(flowID, entity string) (Completion, bool)
	Record(completion Completion)
}

// Deps is the settlement identity plus its collaborators. Snapshot supplies
// the item view read before consume rows are planned; Entity is the actor that
// owns the completion and state-field rows; Tick is recorded verbatim on the
// Completion.
type Deps struct {
	Snapshot condition.Snapshot
	Store    Store
	Ledger   Ledger
	Entity   string
	Tick     int
}
