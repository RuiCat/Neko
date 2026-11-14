// select.go turns one scope's raw event stream into what a push actually
// carries: policy-filtered, merged and budget-limited. Folding the same stream
// into current values gives the pre-step card and the panel snapshot.
package observation

import (
	"encoding/json"
	"sort"
	"time"
)

// Selection is one delta read for a consumer: the events worth pushing since a
// cursor, the cursor to store next, and what the budget dropped.
type Selection struct {
	Events    []Event
	Cursor    int64
	Version   int64
	Truncated bool
	Dropped   int
}

// StateEntry is one path's current value in a scope, older changes folded away.
type StateEntry struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
	From  any    `json:"from"`
	Tier  Tier   `json:"tier"`
	Seq   int64  `json:"seq"`
	At    string `json:"at"`
}

// Select applies the policy to the events after since: noise and sub-threshold
// changes are dropped, repeated normal changes of one path merge inside the
// coalesce window, and the result is ordered critical-first then budget-capped.
func Select(bus *Bus, scope string, since int64, limit int, policy *Policy) Selection {
	version := bus.Version(scope)
	if since > version {
		since = version
	}
	raw := bus.Stream(scope, since, 0)
	budget := policy.BudgetOf()
	window := time.Duration(budget.CoalesceMs) * time.Millisecond

	collected := make([]Event, 0, len(raw))
	positions := make(map[string]int, len(raw))
	dropped := 0
	for _, event := range raw {
		if event.Seeded || event.Path == "" {
			continue
		}
		tier := policy.TierOf(event.Path)
		if tier == TierNoise {
			dropped++
			continue
		}
		if belowThreshold(event, policy.ThresholdOf(event.Path)) {
			dropped++
			continue
		}
		event.Tier = tier
		if position, exists := positions[event.Path]; exists && tier == TierNormal &&
			collected[position].Tier == TierNormal && withinWindow(collected[position].At, event.At, window) {
			collected[position] = mergeEvent(collected[position], event)
			dropped++
			continue
		}
		positions[event.Path] = len(collected)
		collected = append(collected, event)
	}
	sort.SliceStable(collected, func(left, right int) bool {
		if collected[left].Tier.rank() != collected[right].Tier.rank() {
			return collected[left].Tier.rank() < collected[right].Tier.rank()
		}
		return collected[left].Seq < collected[right].Seq
	})
	capacity := budget.MaxItems
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	selection := Selection{Version: version, Dropped: dropped}
	switch {
	case len(collected) > capacity:
		selection.Events = collected[:capacity]
		selection.Truncated = true
	case len(collected) > 0:
		selection.Events = collected
	}
	if len(selection.Events) > 0 {
		selection.Cursor = selection.Events[len(selection.Events)-1].Seq
	} else {
		selection.Cursor = version
	}
	return selection
}

// State folds the whole retained stream into one latest value per path,
// dropping noise, ordered critical-first then newest-first.
func State(bus *Bus, scope string, policy *Policy) ([]StateEntry, int64, bool) {
	version := bus.Version(scope)
	raw := bus.Stream(scope, 0, 0)
	folded := make(map[string]StateEntry, len(raw))
	order := make([]string, 0, len(raw))
	for _, event := range raw {
		if event.Path == "" {
			continue
		}
		tier := policy.TierOf(event.Path)
		if tier == TierNoise {
			continue
		}
		if _, exists := folded[event.Path]; !exists {
			order = append(order, event.Path)
		}
		folded[event.Path] = StateEntry{
			Path:  event.Path,
			Value: event.To,
			From:  event.From,
			Tier:  tier,
			Seq:   event.Seq,
			At:    event.At,
		}
	}
	entries := make([]StateEntry, 0, len(order))
	for _, path := range order {
		entries = append(entries, folded[path])
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].Tier.rank() != entries[right].Tier.rank() {
			return entries[left].Tier.rank() < entries[right].Tier.rank()
		}
		return entries[left].Seq > entries[right].Seq
	})
	budget := policy.BudgetOf()
	truncated := false
	if len(entries) > budget.MaxItems {
		entries = entries[:budget.MaxItems]
		truncated = true
	}
	return entries, version, truncated
}

// mergeEvent folds a newer change of the same path into the earlier candidate:
// the earliest from, the latest to, the highest tier and every rule id.
func mergeEvent(earlier, later Event) Event {
	merged := earlier
	merged.To = later.To
	merged.Seq = later.Seq
	merged.At = later.At
	merged.RuleIDs = mergeRuleIDs(earlier.RuleIDs, later.RuleIDs)
	if later.Tier.rank() < merged.Tier.rank() {
		merged.Tier = later.Tier
	}
	return merged
}

// mergeRuleIDs appends the rule ids that are not present yet, keeping order.
func mergeRuleIDs(current, extra []string) []string {
	if len(extra) == 0 {
		return current
	}
	result := current
	for _, id := range extra {
		found := false
		for _, existing := range result {
			if existing == id {
				found = true
				break
			}
		}
		if !found {
			result = append(result, id)
		}
	}
	return result
}

// withinWindow reports whether two RFC3339 timestamps are within window of each
// other. Unparsable timestamps never merge.
func withinWindow(earlier, later string, window time.Duration) bool {
	if window <= 0 || earlier == "" || later == "" {
		return false
	}
	first, firstErr := time.Parse(time.RFC3339, earlier)
	second, secondErr := time.Parse(time.RFC3339, later)
	if firstErr != nil || secondErr != nil {
		return false
	}
	return second.Sub(first) <= window
}

// belowThreshold reports whether a numeric movement is too small to push.
func belowThreshold(event Event, threshold float64) bool {
	if threshold <= 0 {
		return false
	}
	from, fromOK := toNumber(event.From)
	to, toOK := toNumber(event.To)
	if !fromOK || !toOK {
		return false
	}
	delta := to - from
	if delta < 0 {
		delta = -delta
	}
	return delta < threshold
}

// toNumber reads any JSON-number-ish value as a float64.
func toNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return number, true
	default:
		return 0, false
	}
}
