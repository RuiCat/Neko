// aggregate.go folds TickResult.Transitions into one change list per entity:
// the earliest from, the latest to and deduplicated rule ids (K2).
package eventboard

import (
	"strings"

	"github.com/RuiCat/Neko/internal/engine"
)

// Change is one path's aggregated movement within a single tick.
type Change struct {
	Path    string
	From    any
	To      any
	RuleIDs []string
}

// entityChanges is one entity's ordered changes from a tick.
type entityChanges struct {
	Entity  string
	Changes []Change
}

// aggregateTransitions groups a tick's transitions by entity and folds each
// entity's `set` operations into one Change per path. A repeated path keeps the
// earliest from, the latest to and one copy of every rule id. Non-set
// operations (add/append/emit) and empty paths are ignored: they are not state
// path replacements the board can render as from -> to.
func aggregateTransitions(transitions []engine.Transition) []entityChanges {
	groups := make([]entityChanges, 0, len(transitions))
	groupPositions := make(map[string]int, len(transitions))
	pathPositions := make([]map[string]int, 0, len(transitions))
	for _, transition := range transitions {
		if transition.EntityID == "" {
			continue
		}
		for _, operation := range transition.Operations {
			if readString(operation, "op") != "set" {
				continue
			}
			path := readString(operation, "path")
			if path == "" {
				continue
			}
			groupIndex, exists := groupPositions[transition.EntityID]
			if !exists {
				groupIndex = len(groups)
				groupPositions[transition.EntityID] = groupIndex
				groups = append(groups, entityChanges{Entity: transition.EntityID})
				pathPositions = append(pathPositions, map[string]int{})
			}
			value := copyValue(operation["value"])
			changes := &groups[groupIndex].Changes
			indexes := pathPositions[groupIndex]
			if position, found := indexes[path]; found {
				(*changes)[position].To = value
				(*changes)[position].RuleIDs = appendRuleID((*changes)[position].RuleIDs, transition.RuleID)
				continue
			}
			indexes[path] = len(*changes)
			*changes = append(*changes, Change{
				Path:    path,
				From:    lookupFrom(transition.FromState, path),
				To:      value,
				RuleIDs: ruleIDs(transition.RuleID),
			})
		}
	}
	return groups
}

// readString returns operation[key] as a string, or "" when absent/non-string.
func readString(operation map[string]any, key string) string {
	value, _ := operation[key].(string)
	return value
}

// ruleIDs wraps one rule id; an empty id keeps the list empty.
func ruleIDs(ruleID string) []string {
	if ruleID == "" {
		return []string{}
	}
	return []string{ruleID}
}

// appendRuleID adds a rule id once, preserving the first-seen order.
func appendRuleID(current []string, ruleID string) []string {
	if ruleID == "" {
		return current
	}
	for _, existing := range current {
		if existing == ruleID {
			return current
		}
	}
	return append(current, ruleID)
}

// lookupFrom resolves a dotted path against the transition's previous-state
// snapshot. The engine stores that snapshot as the state group's *content*
// ({"心情":{"当前":"满足"}}) while operation paths start at the entity root
// ("状态.心情.当前"), so a direct dotted key is tried first and then the path is
// walked from every segment offset: dropping leading segments lines the two
// roots up without hard-coding the configured state-group name. A path absent
// from the snapshot yields nil, which the board reports as null.
func lookupFrom(fromState map[string]any, path string) any {
	if value, exists := fromState[path]; exists {
		return copyValue(value)
	}
	segments := strings.Split(path, ".")
	for offset := 0; offset < len(segments); offset++ {
		if value, found := walkPath(fromState, segments[offset:]); found {
			return copyValue(value)
		}
	}
	return nil
}

// walkPath walks a segment list down nested maps, reporting whether it landed.
func walkPath(root map[string]any, segments []string) (any, bool) {
	var current any = root
	for _, segment := range segments {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := object[segment]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

// copyValue deep-copies maps and slices so a board never aliases engine state
// that a later tick may replace.
func copyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = copyValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = copyValue(item)
		}
		return out
	default:
		return value
	}
}
