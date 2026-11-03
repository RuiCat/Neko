// Package expert defines the five memory-type experts and their isolated
// contexts (C5). Experts only read the board and write their own context; they
// never generate answers and never touch hard state.
package expert

import (
	"context"
	"fmt"

	"github.com/RuiCat/Neko/internal/blackboard"
)

// Entry is one remembered item in an expert's private context. SnapshotID is
// the anchor that later lets a fragment be traced back to a hard-state capture.
type Entry struct {
	SnapshotID string
	Kind       string
	Content    string
	Weight     float64
	At         int64
}

// Input is what an expert may read for one turn: the user text and the shared
// board. Its own context is internal to the expert.
type Input struct {
	UserInput string
	Board     blackboard.Reader
}

// Fragment is one piece of memory compensation. Type is "retrieved" or
// "reconstructed"; reconstructed items must carry a lower weight.
type Fragment struct {
	Content          string
	SourceSnapshotID string
	Type             string
	Weight           float64
}

// Expert produces memory fragments for its category.
type Expert interface {
	ID() string
	Compensate(ctx context.Context, in Input) ([]Fragment, error)
}

// Registry holds experts by ID and preserves activation order.
type Registry struct {
	experts map[string]Expert
	order   []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{experts: map[string]Expert{}}
}

// Add registers one expert; duplicate IDs are rejected.
func (r *Registry) Add(entry Expert) error {
	if entry == nil {
		return fmt.Errorf("expert: 不能注册 nil")
	}
	id := entry.ID()
	if _, exists := r.experts[id]; exists {
		return fmt.Errorf("expert: 重复注册 %q", id)
	}
	r.experts[id] = entry
	r.order = append(r.order, id)
	return nil
}

// Get returns one expert by ID.
func (r *Registry) Get(id string) (Expert, bool) {
	entry, ok := r.experts[id]
	return entry, ok
}

// Remember stores one entry in the named expert's private context and reports
// whether the expert accepted it (unknown IDs and non-recall experts return
// false).
func (r *Registry) Remember(id string, entry Entry) bool {
	expert, ok := r.experts[id]
	if !ok {
		return false
	}
	rememberer, ok := expert.(interface{ Remember(Entry) })
	if !ok {
		return false
	}
	rememberer.Remember(entry)
	return true
}

// IDs returns the registration order.
func (r *Registry) IDs() []string {
	return append([]string(nil), r.order...)
}

// DecayAll runs the nightly decay on every expert that supports it and returns
// how many entries each one dropped.
func (r *Registry) DecayAll(factor float64) map[string]int {
	removed := map[string]int{}
	for id, expert := range r.experts {
		decayable, ok := expert.(interface{ Decay(float64) int })
		if !ok {
			continue
		}
		removed[id] = decayable.Decay(factor)
	}
	return removed
}

// Snapshot returns a copy of every expert context (for persistence).
func (r *Registry) Snapshot() map[string][]Entry {
	snapshot := map[string][]Entry{}
	for id, expert := range r.experts {
		entries, ok := expert.(interface{ Entries() []Entry })
		if !ok {
			continue
		}
		snapshot[id] = entries.Entries()
	}
	return snapshot
}

// Load restores contexts saved by Snapshot. Unknown experts are ignored.
func (r *Registry) Load(snapshot map[string][]Entry) {
	for id, entries := range snapshot {
		expert, ok := r.experts[id]
		if !ok {
			continue
		}
		replaceable, ok := expert.(interface{ Replace([]Entry) })
		if !ok {
			continue
		}
		replaceable.Replace(entries)
	}
}

// Compensate runs the active experts in the given order and concatenates their
// fragments. An unknown ID is an error: a gate/registry mismatch must not be
// silently swallowed.
func (r *Registry) Compensate(ctx context.Context, active []string, in Input) ([]Fragment, error) {
	fragments := make([]Fragment, 0, len(active))
	for _, id := range active {
		entry, ok := r.experts[id]
		if !ok {
			return nil, fmt.Errorf("expert: 未注册的专家 %q", id)
		}
		part, err := entry.Compensate(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("expert %q: %w", id, err)
		}
		fragments = append(fragments, part...)
	}
	return fragments, nil
}
