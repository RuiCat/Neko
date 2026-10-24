// Conflict construction and same-tick target-state race detection.
//
// Python's engine only emits "no_branch_match" conflicts. The Go port also
// records the spec-mandated "branch_race" conflict: two rules in the same tick
// drive one entity's state group to different values (including writes made by
// @函数/事件 handlers).
package engine

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// targetWrite records the latest state-group target written by a rule.
type targetWrite struct {
	ruleID     string
	value      any
	conflicted bool
}

func newConflictID() string {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return "conflict"
}

func (m *StateMachine) makeConflict(ent *entity.Entity, rule Rule, conflictType, reason string) Conflict {
	state := stateSnapshot(ent)
	context := map[string]any{
		"entity":                    ent.ID,
		"type":                      ent.Type,
		config.Current().StateGroup: state,
	}
	var candidates []any
	stateDefinitions, ok := ent.GetOrNone(config.Current().StateDefinitionField).(map[string]any)
	if ok {
		groups := make([]string, 0, len(stateDefinitions))
		for group := range stateDefinitions {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		for _, group := range groups {
			states, ok := stateDefinitions[group].([]any)
			if !ok {
				continue
			}
			current := state[group]
			for _, candidate := range states {
				if !equalValues(candidate, current) {
					candidates = append(candidates, candidate)
				}
			}
		}
	}
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	if candidates == nil {
		candidates = []any{}
	}
	return Conflict{
		ConflictID: newConflictID(),
		EntityID:   ent.ID,
		RuleID:     rule.ID,
		Type:       conflictType,
		Context:    context,
		Candidates: candidates,
		Reason:     reason,
	}
}

// noteStateWrite is the per-tick change listener: it records the latest
// state-group target written by the currently executing rule and raises a
// branch_race when two different rules disagree about the same group.
func (m *StateMachine) noteStateWrite(ent *entity.Entity, path string, value any) {
	if m.tickResult == nil || m.tickTracker == nil || m.currentRule == "" {
		return
	}
	prefix := config.Current().StateGroup + "."
	if !strings.HasPrefix(path, prefix) {
		return
	}
	group := strings.TrimPrefix(path, prefix)
	if group == "" || strings.Contains(group, ".") {
		return
	}
	stateDefinitions, _ := ent.GetOrNone(config.Current().StateDefinitionField).(map[string]any)
	if _, declared := stateDefinitions[group]; !declared {
		return
	}
	writes, ok := m.tickTracker[ent.ID]
	if !ok {
		writes = map[string]targetWrite{}
		m.tickTracker[ent.ID] = writes
	}
	previous, exists := writes[group]
	if exists && !previous.conflicted && previous.ruleID != m.currentRule && !equalValues(previous.value, value) {
		conflict := m.makeConflict(ent, Rule{ID: m.currentRule}, "branch_race",
			"规则 '"+previous.ruleID+"' 与 '"+m.currentRule+"' 对 "+config.Current().StateGroup+"."+group+" 的目标值不同")
		conflict.Candidates = []any{previous.value, value}
		m.tickResult.Conflicts = append(m.tickResult.Conflicts, conflict)
		m.pendingConflicts[conflict.ConflictID] = conflict
		m.tickResult.Status = "conflict"
		previous.conflicted = true
	}
	writes[group] = targetWrite{ruleID: m.currentRule, value: value, conflicted: previous.conflicted}
}
