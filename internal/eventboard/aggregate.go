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
	// FromKnown 标记 From 真的来自上一状态。状态组之外的路径在 transition 的
	// FromState 里查不到，此时 From 为 nil、FromKnown=false，board 会改用该条目
	// 上一次的 To 作为 from，避免每次都显示 "null → 值"。直接写入（MCP/控制面）
	// 能读到写入前的真实值，因此总是 FromKnown=true。
	FromKnown bool
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
			from, fromKnown := lookupFrom(transition.FromState, path)
			*changes = append(*changes, Change{
				Path:      path,
				From:      from,
				To:        value,
				RuleIDs:   ruleIDs(transition.RuleID),
				FromKnown: fromKnown,
			})
		}
	}
	return groups
}

// aggregateWrites folds the tick's observed writes (rule ops, text effects and
// implicit @函数/事件 writes) into per-entity changes: earliest from, latest to
// and deduplicated rule ids. Paths whose net change is zero are skipped; the
// scene event stream is kept here and bounded later by the board projection
// (sceneEventProjection.go) so the push never carries the whole list.
func aggregateWrites(writes []engine.EntityWrite) []entityChanges {
	groups := make([]entityChanges, 0, len(writes))
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
			groups = append(groups, entityChanges{Entity: write.EntityID})
			pathPositions = append(pathPositions, map[string]int{})
		}
		changes := &groups[groupIndex].Changes
		indexes := pathPositions[groupIndex]
		if position, found := indexes[write.Path]; found {
			(*changes)[position].To = copyValue(write.To)
			(*changes)[position].RuleIDs = appendRuleID((*changes)[position].RuleIDs, write.RuleID)
			continue
		}
		indexes[write.Path] = len(*changes)
		*changes = append(*changes, Change{
			Path:      write.Path,
			From:      copyValue(write.From),
			To:        copyValue(write.To),
			RuleIDs:   ruleIDs(write.RuleID),
			FromKnown: true,
		})
	}
	// 同 tick 内改了又改回去的路径没有净变化，不记黑板。
	nonEmpty := groups[:0]
	for _, group := range groups {
		kept := group.Changes[:0]
		for _, change := range group.Changes {
			if valuesEqual(change.From, change.To) {
				continue
			}
			kept = append(kept, change)
		}
		group.Changes = kept
		if len(kept) > 0 {
			nonEmpty = append(nonEmpty, group)
		}
	}
	return nonEmpty
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
func lookupFrom(fromState map[string]any, path string) (any, bool) {
	if value, exists := fromState[path]; exists {
		return copyValue(value), true
	}
	segments := strings.Split(path, ".")
	for offset := 0; offset < len(segments); offset++ {
		if value, found := walkPath(fromState, segments[offset:]); found {
			return copyValue(value), true
		}
	}
	return nil, false
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
