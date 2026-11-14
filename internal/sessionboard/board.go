// board.go is one session board's data model: regions derived from the caller,
// per-key entries with from/to, one version bump per changed batch and the
// reader views/deltas of K1/K4. It holds no lock; the store serializes access.
package sessionboard

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

const (
	// SCOPE_WORKSPACE addresses the board of one workspace (K1).
	SCOPE_WORKSPACE = "workspace"
	// SCOPE_GLOBAL addresses the one board shared by every workspace (K1).
	SCOPE_GLOBAL = "global"
	// GLOBAL_SCOPE_ID is the scope_id of the global board (K1).
	GLOBAL_SCOPE_ID = "*"
	// MAX_REGIONS bounds one board; the 65th region evicts the oldest (K3).
	MAX_REGIONS = 64
	// MAX_REGION_ENTRIES bounds one region, tombstones included (K3).
	MAX_REGION_ENTRIES = 200
	// MAX_BOARD_BYTES bounds one board's serialized size (K9).
	MAX_BOARD_BYTES = 256 * 1024
)

// Owner is who controls one region (K1): a session on a workspace board, a
// workspace on the global board.
type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Entry is one key's latest movement inside a region (K1). A delete rewrites
// the entry as a tombstone (to null, deleted true) instead of removing it.
type Entry struct {
	Key       string `json:"key"`
	From      any    `json:"from"`
	To        any    `json:"to"`
	By        string `json:"by"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Deleted   bool   `json:"deleted"`
}

// Region is one caller's slice of a board: its identity, its last version and
// its entries (K1).
type Region struct {
	ID        string  `json:"id"`
	Owner     Owner   `json:"owner"`
	Version   int     `json:"version"`
	UpdatedAt string  `json:"updated_at"`
	Entries   []Entry `json:"entries"`
}

// Board is one level's whole board (K1).
type Board struct {
	Scope              string   `json:"scope"`
	ScopeID            string   `json:"scope_id"`
	Version            int      `json:"version"`
	UpdatedAt          string   `json:"updated_at"`
	MinRetainedVersion int      `json:"min_retained_version"`
	Regions            []Region `json:"regions"`
}

// Delta is one entry newer than a reader's cursor (K4). It carries the region
// id so the bridge can label a line without a second lookup.
type Delta struct {
	RegionID  string `json:"region_id"`
	Key       string `json:"key"`
	From      any    `json:"from"`
	To        any    `json:"to"`
	By        string `json:"by"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Deleted   bool   `json:"deleted"`
}

// newBoard returns the empty baseline board of one scope (K1).
func newBoard(scope, scopeID string) Board {
	return Board{Scope: scope, ScopeID: scopeID, Regions: []Region{}}
}

// EmptyBoard returns the version-0 view of a scope, used for unknown
// workspaces and for embedders without a store (K4: empty board, never 404).
func EmptyBoard(scope, workspace string) Board {
	scopeID := GLOBAL_SCOPE_ID
	if scope == SCOPE_WORKSPACE {
		scopeID = workspace
	}
	return newBoard(scope, scopeID)
}

// clone returns a deep copy so a reader or a candidate write never aliases the
// store's live board.
func (b *Board) clone() *Board {
	out := &Board{Scope: b.Scope, ScopeID: b.ScopeID, Version: b.Version, UpdatedAt: b.UpdatedAt,
		MinRetainedVersion: b.MinRetainedVersion, Regions: make([]Region, len(b.Regions))}
	for index, region := range b.Regions {
		copied := Region{ID: region.ID, Owner: region.Owner, Version: region.Version,
			UpdatedAt: region.UpdatedAt, Entries: make([]Entry, len(region.Entries))}
		for position, entry := range region.Entries {
			copied.Entries[position] = cloneEntry(entry)
		}
		out.Regions[index] = copied
	}
	return out
}

// liveView copies the board for a reader: tombstones stay out of entries and
// live values never alias the store (K4).
func (b *Board) liveView() Board {
	out := Board{Scope: b.Scope, ScopeID: b.ScopeID, Version: b.Version, UpdatedAt: b.UpdatedAt,
		MinRetainedVersion: b.MinRetainedVersion, Regions: make([]Region, 0, len(b.Regions))}
	for _, region := range b.Regions {
		copied := Region{ID: region.ID, Owner: region.Owner, Version: region.Version,
			UpdatedAt: region.UpdatedAt, Entries: make([]Entry, 0, len(region.Entries))}
		for _, entry := range region.Entries {
			if entry.Deleted {
				continue
			}
			copied.Entries = append(copied.Entries, cloneEntry(entry))
		}
		out.Regions = append(out.Regions, copied)
	}
	return out
}

// deltasSince lists every entry written after the cursor, tombstones included,
// in ascending version order (K4). Two entries of one batch share a version and
// keep their region order.
func (b *Board) deltasSince(since int) []Delta {
	deltas := []Delta{}
	for _, region := range b.Regions {
		for _, entry := range region.Entries {
			if entry.Version <= since {
				continue
			}
			deltas = append(deltas, Delta{RegionID: region.ID, Key: entry.Key, From: copyValue(entry.From),
				To: copyValue(entry.To), By: entry.By, Version: entry.Version, UpdatedAt: entry.UpdatedAt,
				Deleted: entry.Deleted})
		}
	}
	sort.SliceStable(deltas, func(i, j int) bool { return deltas[i].Version < deltas[j].Version })
	return deltas
}

// minRetainedVersion is the smallest version still retained; zero when the
// board has no entries at all (K1).
func (b *Board) minRetainedVersion() int {
	smallest := 0
	for _, region := range b.Regions {
		for _, entry := range region.Entries {
			if smallest == 0 || entry.Version < smallest {
				smallest = entry.Version
			}
		}
	}
	return smallest
}

// refreshMinRetained recomputes the retained-version watermark after a change
// or a load (K1).
func (b *Board) refreshMinRetained() {
	b.MinRetainedVersion = b.minRetainedVersion()
}

// regionIndex finds one region by id, or -1 when the board has none.
func (b *Board) regionIndex(regionID string) int {
	for index := range b.Regions {
		if b.Regions[index].ID == regionID {
			return index
		}
	}
	return -1
}

// entryIndex finds one entry by key inside a region, live or tombstone (K1: a
// key is unique per region across both states).
func (r *Region) entryIndex(key string) int {
	for index := range r.Entries {
		if r.Entries[index].Key == key {
			return index
		}
	}
	return -1
}

// applyOps folds one validated batch into the caller's region and reports
// whether the board changed plus the keys dropped by the entry cap (K3/K5).
// A no-op batch neither creates a region nor bumps the version.
func (b *Board) applyOps(target Target, by string, ops []writeOp, at time.Time) (bool, []string) {
	stamp := at.Format(time.RFC3339)
	next := b.Version + 1
	index := b.regionIndex(target.RegionID)
	created := index < 0
	if created {
		b.Regions = append(b.Regions, Region{ID: target.RegionID, Owner: target.Owner, Entries: []Entry{}})
		index = len(b.Regions) - 1
	}
	region := &b.Regions[index]
	changed := false
	for _, op := range ops {
		switch op.kind {
		case OP_SET:
			changed = applySet(region, op, by, next, stamp) || changed
		case OP_DELETE:
			changed = applyDelete(region, op.key, by, next, stamp) || changed
		case OP_CLEAR:
			changed = applyClear(region, by, next, stamp) || changed
		}
	}
	if !changed {
		if created {
			b.Regions = b.Regions[:len(b.Regions)-1]
		}
		return false, nil
	}
	b.Version = next
	b.UpdatedAt = stamp
	region.Version = next
	region.UpdatedAt = stamp
	trimmed := region.trimEntries()
	b.refreshMinRetained()
	return true, trimmed
}

// applySet writes one key, keeping the previous value as from (null when the
// key had no live value). Rewriting the same value is not a change (K5).
func applySet(region *Region, op writeOp, by string, version int, stamp string) bool {
	position := region.entryIndex(op.key)
	if position >= 0 {
		entry := &region.Entries[position]
		if !entry.Deleted && valuesEqual(entry.To, op.value) {
			return false
		}
		entry.From = nil
		if !entry.Deleted {
			entry.From = copyValue(entry.To)
		}
		entry.To = copyValue(op.value)
		entry.By = by
		entry.Version = version
		entry.UpdatedAt = stamp
		entry.Deleted = false
		return true
	}
	region.Entries = append(region.Entries, Entry{Key: op.key, From: nil, To: copyValue(op.value),
		By: by, Version: version, UpdatedAt: stamp, Deleted: false})
	return true
}

// applyDelete rewrites one live key as a tombstone. A missing or already
// deleted key is not a change (K5).
func applyDelete(region *Region, key, by string, version int, stamp string) bool {
	position := region.entryIndex(key)
	if position < 0 {
		return false
	}
	entry := &region.Entries[position]
	if entry.Deleted {
		return false
	}
	entry.From = copyValue(entry.To)
	entry.To = nil
	entry.By = by
	entry.Version = version
	entry.UpdatedAt = stamp
	entry.Deleted = true
	return true
}

// applyClear rewrites every live key of the region as a tombstone. An already
// empty region is not a change (K5).
func applyClear(region *Region, by string, version int, stamp string) bool {
	changed := false
	for index := range region.Entries {
		entry := &region.Entries[index]
		if entry.Deleted {
			continue
		}
		entry.From = copyValue(entry.To)
		entry.To = nil
		entry.By = by
		entry.Version = version
		entry.UpdatedAt = stamp
		entry.Deleted = true
		changed = true
	}
	return changed
}

// trimEntries enforces MAX_REGION_ENTRIES by dropping the oldest updated_at
// entries; on a tie the earlier entry in insertion order goes first. It reports
// the dropped keys in drop order (K3 trimmed). Timestamps written by one process
// share one RFC3339 offset, so string order matches time order (a file with
// mixed offsets is abnormal data, not a supported cap input).
func (r *Region) trimEntries() []string {
	if len(r.Entries) <= MAX_REGION_ENTRIES {
		return nil
	}
	ordered := make([]Entry, len(r.Entries))
	copy(ordered, r.Entries)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].UpdatedAt < ordered[j].UpdatedAt })
	dropCount := len(ordered) - MAX_REGION_ENTRIES
	dropped := make(map[string]struct{}, dropCount)
	trimmed := make([]string, 0, dropCount)
	for _, entry := range ordered[:dropCount] {
		dropped[entry.Key] = struct{}{}
		trimmed = append(trimmed, entry.Key)
	}
	kept := r.Entries[:0]
	for _, entry := range r.Entries {
		if _, gone := dropped[entry.Key]; gone {
			continue
		}
		kept = append(kept, entry)
	}
	r.Entries = kept
	return trimmed
}

// enforceRegionCap evicts the oldest regions past MAX_REGIONS and reports the
// evicted region ids (K3). The just-written region is the newest, so a normal
// write never evicts itself; the insertion order breaks timestamp ties.
func (b *Board) enforceRegionCap() []string {
	if len(b.Regions) <= MAX_REGIONS {
		return nil
	}
	order := make([]int, len(b.Regions))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool {
		return b.Regions[order[i]].UpdatedAt < b.Regions[order[j]].UpdatedAt
	})
	dropCount := len(b.Regions) - MAX_REGIONS
	evicted := make([]string, 0, dropCount)
	dropped := make(map[string]struct{}, dropCount)
	for _, index := range order[:dropCount] {
		regionID := b.Regions[index].ID
		dropped[regionID] = struct{}{}
		evicted = append(evicted, regionID)
	}
	kept := b.Regions[:0]
	for _, region := range b.Regions {
		if _, gone := dropped[region.ID]; gone {
			continue
		}
		kept = append(kept, region)
	}
	b.Regions = kept
	b.refreshMinRetained()
	return evicted
}

// encodeSize reports the board's serialized size before a write commits (K9).
func (b *Board) encodeSize() (int, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}
	return len(raw), nil
}

// cloneEntry copies one entry so from/to never alias stored JSON values.
func cloneEntry(entry Entry) Entry {
	return Entry{Key: entry.Key, From: copyValue(entry.From), To: copyValue(entry.To), By: entry.By,
		Version: entry.Version, UpdatedAt: entry.UpdatedAt, Deleted: entry.Deleted}
}

// copyValue deeply copies one JSON-shaped value so no two boards share maps or
// slices (JSON round-trip keeps numbers as float64, like the stored form).
func copyValue(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var copied any
	if err := json.Unmarshal(raw, &copied); err != nil {
		return nil
	}
	return copied
}

// valuesEqual compares two JSON-shaped values by their encoded form, so
// numbers of different Go types still compare equal (K5 same-value rewrite).
func valuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return bytes.Equal(leftJSON, rightJSON)
}
