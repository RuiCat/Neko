// recall.go is the bundled rule-based expert: it recalls the newest entries of
// its own category and returns them as retrieved fragments. It is deliberately
// simple; C5 allows replacing it with a trained expert behind the same
// interface.
package expert

import (
	"context"

	"github.com/RuiCat/Neko/internal/gate"
)

// DEFAULT_RECALL_FRAGMENTS is how many entries one expert contributes per turn.
const DEFAULT_RECALL_FRAGMENTS = 2

// RecallExpert returns the newest entries of one memory category.
type RecallExpert struct {
	id  string
	max int
	ctx *Context
}

// NewRecallExpert builds one expert. capacity bounds its private context and
// max bounds the fragments returned per turn.
func NewRecallExpert(id string, capacity, max int) *RecallExpert {
	if max <= 0 {
		max = DEFAULT_RECALL_FRAGMENTS
	}
	return &RecallExpert{id: id, max: max, ctx: NewContext(capacity)}
}

// ID returns the expert ID (the same string the gate activates).
func (e *RecallExpert) ID() string { return e.id }

// Context exposes the private context to the nightly decay pass and tests.
func (e *RecallExpert) Context() *Context { return e.ctx }

// Remember stores one entry in the private context.
func (e *RecallExpert) Remember(entry Entry) {
	e.ctx.Remember(entry)
}

// Decay applies the nightly weight decay and returns how many entries dropped.
func (e *RecallExpert) Decay(factor float64) int {
	return e.ctx.Decay(factor)
}

// Entries returns a copy of the private context (persistence).
func (e *RecallExpert) Entries() []Entry {
	return e.ctx.Recent(0)
}

// Replace restores the private context from a persisted snapshot.
func (e *RecallExpert) Replace(entries []Entry) {
	e.ctx.Replace(entries)
}

// Compensate returns the newest entries as retrieved fragments. No entries
// means no fragments (the assembler decides the fallback, not the expert).
func (e *RecallExpert) Compensate(ctx context.Context, in Input) ([]Fragment, error) {
	entries := e.ctx.Recent(e.max)
	fragments := make([]Fragment, 0, len(entries))
	for _, entry := range entries {
		fragments = append(fragments, Fragment{
			Content:          entry.Content,
			SourceSnapshotID: entry.SnapshotID,
			Type:             "retrieved",
			Weight:           entry.Weight,
		})
	}
	return fragments, nil
}

// NewDefaultRegistry builds the five bundled experts keyed by the gate IDs.
func NewDefaultRegistry(capacity int) *Registry {
	registry := NewRegistry()
	for _, id := range gate.DEFAULT_EXPERT_ORDER {
		_ = registry.Add(NewRecallExpert(id, capacity, DEFAULT_RECALL_FRAGMENTS))
	}
	return registry
}
