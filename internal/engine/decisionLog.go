package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SetDecisionLog enables persistence of arbitration decisions (JSON array).
func (m *StateMachine) SetDecisionLog(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisionPath = path
}

// logDecision appends one decision record to llm_decisions.json. Called with
// m.mu held.
func (m *StateMachine) logDecision(conflict Conflict, target map[string]any, reason string) error {
	records := []map[string]any{}
	if raw, err := os.ReadFile(m.decisionPath); err == nil {
		_ = json.Unmarshal(raw, &records)
	}
	records = append(records, map[string]any{
		"conflict_id":  conflict.ConflictID,
		"entity":       conflict.EntityID,
		"rule_id":      conflict.RuleID,
		"type":         conflict.Type,
		"context":      conflict.Context,
		"target_state": target,
		"reason":       reason,
		"timestamp":    time.Now().Format("2006-01-02T15:04:05"),
	})
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.decisionPath), 0o755); err != nil {
		return err
	}
	tmp := m.decisionPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.decisionPath)
}

// backfillRuleLocked turns a resolved decision into a high-priority learned
// rule. Called with m.mu held.
func (m *StateMachine) backfillRuleLocked(conflict Conflict, target map[string]any, effects []Op) {
	groups := make([]string, 0, len(target))
	for group := range target {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	then := make([]Op, 0, len(groups)+len(effects))
	for _, group := range groups {
		then = append(then, Op{
			Op: "set", Path: conflict.EntityID + ".状态." + group, Value: target[group],
		})
	}
	then = append(then, effects...)
	priority := 100
	for _, rule := range m.rules {
		if rule.ID == conflict.RuleID {
			priority = rule.Priority + 100
			break
		}
	}
	empty := ""
	m.addRuleLocked(Rule{
		ID: "learned_" + conflict.ConflictID, Match: conflict.EntityID,
		Branches: []Branch{{If: &empty, Then: then}},
		Priority: priority, Source: "llm_backfill", Enabled: true,
		Desc: "LLM 回填: " + conflict.EntityID + " (" + conflict.Reason + ")",
	})
}
