// board.go is one entity's change board: per-path entries updated in place, a
// version bumped once per changed tick and a hard entry cap (K1 data model).
package eventboard

import (
	"sort"
	"time"
)

// MAX_ENTRIES bounds one entity's board. It only defends against abnormal data;
// a normal world stays far below it.
const MAX_ENTRIES = 200

// Entry records one state path's latest movement on a board.
type Entry struct {
	Path      string   `json:"path"`
	From      any      `json:"from"`
	To        any      `json:"to"`
	RuleIDs   []string `json:"rule_ids"`
	UpdatedAt string   `json:"updated_at"`
}

// Board is one entity's change board (K1): at most one entry per path, entries
// are never removed by an update and the version grows once per changed tick.
type Board struct {
	Entity    string  `json:"entity"`
	Version   int     `json:"version"`
	UpdatedAt string  `json:"updated_at"`
	TickID    string  `json:"tick_id"`
	Entries   []Entry `json:"entries"`
}

// newBoard returns the empty baseline board of one entity: no entries and no
// version, so the first observation only fixes that the entity exists.
func newBoard(entity string) *Board {
	return &Board{Entity: entity, Entries: []Entry{}}
}

// apply folds one tick's changes into the board in place. A path that already
// has an entry updates it (from/to/rule_ids/updated_at) instead of appending;
// untouched paths keep their entries. The version grows once for the whole
// tick, never once per path. It reports whether the board changed.
func (b *Board) apply(changes []Change, tickID string, at time.Time) bool {
	if len(changes) == 0 {
		return false
	}
	stamp := at.Format(time.RFC3339)
	positions := make(map[string]int, len(b.Entries)+len(changes))
	for index, entry := range b.Entries {
		positions[entry.Path] = index
	}
	for _, change := range changes {
		if index, exists := positions[change.Path]; exists {
			b.Entries[index].From = copyValue(change.From)
			b.Entries[index].To = copyValue(change.To)
			b.Entries[index].RuleIDs = append([]string{}, change.RuleIDs...)
			b.Entries[index].UpdatedAt = stamp
			continue
		}
		positions[change.Path] = len(b.Entries)
		b.Entries = append(b.Entries, Entry{
			Path:      change.Path,
			From:      copyValue(change.From),
			To:        copyValue(change.To),
			RuleIDs:   append([]string{}, change.RuleIDs...),
			UpdatedAt: stamp,
		})
	}
	b.trim()
	b.Version++
	b.UpdatedAt = stamp
	b.TickID = tickID
	return true
}

// trim enforces MAX_ENTRIES by dropping the oldest updated_at entries; on a tie
// the earlier entry in insertion order goes first. Timestamps written by one
// process share one RFC3339 offset, so string order matches time order (a file
// with mixed offsets is abnormal data, not a supported cap input).
func (b *Board) trim() {
	if len(b.Entries) <= MAX_ENTRIES {
		return
	}
	ordered := make([]Entry, len(b.Entries))
	copy(ordered, b.Entries)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].UpdatedAt < ordered[j].UpdatedAt })
	drop := make(map[string]struct{}, len(ordered)-MAX_ENTRIES)
	for _, entry := range ordered[:len(ordered)-MAX_ENTRIES] {
		drop[entry.Path] = struct{}{}
	}
	kept := b.Entries[:0]
	for _, entry := range b.Entries {
		if _, gone := drop[entry.Path]; gone {
			continue
		}
		kept = append(kept, entry)
	}
	b.Entries = kept
}

// clone returns a deep copy so readers never alias the registry's live board.
func (b *Board) clone() *Board {
	out := &Board{Entity: b.Entity, Version: b.Version, UpdatedAt: b.UpdatedAt, TickID: b.TickID}
	out.Entries = make([]Entry, len(b.Entries))
	for index, entry := range b.Entries {
		out.Entries[index] = entry
		out.Entries[index].From = copyValue(entry.From)
		out.Entries[index].To = copyValue(entry.To)
		out.Entries[index].RuleIDs = append([]string{}, entry.RuleIDs...)
	}
	return out
}
