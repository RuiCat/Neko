// fromTick.go folds one tick's observed writes into observation events: the
// earliest from, the latest to, deduplicated rule ids and no net-zero paths.
package observation

import (
	"encoding/json"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
)

// Change is one path's net movement, independent of where it came from (a
// tick's observed writes or a direct control-plane write).
type Change struct {
	Path    string
	From    any
	To      any
	RuleIDs []string
}

// EntityChanges is one entity's ordered net changes.
type EntityChanges struct {
	Entity  string
	Changes []Change
}

// ChangesFromWrites folds a tick's observed writes per entity. A path written
// several times in one tick keeps its earliest from and latest to; a path that
// returns to its starting value is dropped because it carries no net change.
func ChangesFromWrites(writes []engine.EntityWrite) []EntityChanges {
	groups := make([]EntityChanges, 0, len(writes))
	groupPositions := make(map[string]int, len(writes))
	pathPositions := make([]map[string]int, 0, len(writes))
	for _, write := range writes {
		if write.EntityID == "" || write.Path == "" {
			continue
		}
		groupIndex, exists := groupPositions[write.EntityID]
		if !exists {
			groupIndex = len(groups)
			groupPositions[write.EntityID] = groupIndex
			groups = append(groups, EntityChanges{Entity: write.EntityID})
			pathPositions = append(pathPositions, map[string]int{})
		}
		changes := &groups[groupIndex].Changes
		indexes := pathPositions[groupIndex]
		if position, found := indexes[write.Path]; found {
			(*changes)[position].To = write.To
			(*changes)[position].RuleIDs = mergeRuleIDs((*changes)[position].RuleIDs, ruleIDs(write.RuleID))
			continue
		}
		indexes[write.Path] = len(*changes)
		*changes = append(*changes, Change{
			Path:    write.Path,
			From:    write.From,
			To:      write.To,
			RuleIDs: ruleIDs(write.RuleID),
		})
	}
	kept := groups[:0]
	for _, group := range groups {
		changes := group.Changes[:0]
		for _, change := range group.Changes {
			if valuesEqual(change.From, change.To) {
				continue
			}
			changes = append(changes, change)
		}
		group.Changes = changes
		if len(changes) > 0 {
			kept = append(kept, group)
		}
	}
	return kept
}

// EventsForEntity turns one entity's net changes into events for its scope.
func EventsForEntity(entityID string, changes []Change, tickID string, at time.Time) []Event {
	scope := ScopeForEntity(entityID)
	stamp := at.UTC().Format(time.RFC3339)
	events := make([]Event, 0, len(changes))
	for _, change := range changes {
		if change.Path == "" || valuesEqual(change.From, change.To) {
			continue
		}
		events = append(events, Event{
			Scope:   scope,
			Path:    change.Path,
			From:    change.From,
			To:      change.To,
			RuleIDs: change.RuleIDs,
			TickID:  tickID,
			At:      stamp,
		})
	}
	return events
}

// ruleIDs wraps one rule id; an empty id keeps the list empty.
func ruleIDs(ruleID string) []string {
	if ruleID == "" {
		return nil
	}
	return []string{ruleID}
}

// valuesEqual compares two values by their canonical JSON encoding, so int(1)
// and float64(1) that print the same stay equal.
func valuesEqual(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftRaw) == string(rightRaw)
}
