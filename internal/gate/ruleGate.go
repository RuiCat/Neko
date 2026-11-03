// ruleGate.go is the deterministic v1 gate: keyword/affect rules score the five
// experts, and the top two or three are returned in a stable order. It exists
// so the online loop can run before any model is trained; C5 keeps the
// interface replaceable.
package gate

import (
	"sort"
	"strings"
)

// MIN_ACTIVE_EXPERTS is the lower bound the assembler relies on.
const MIN_ACTIVE_EXPERTS = 2

// MAX_ACTIVE_EXPERTS is the upper bound the context budget allows.
const MAX_ACTIVE_EXPERTS = 3

// RULE_GATE_VERSION identifies the bundled rule set in journal records.
const RULE_GATE_VERSION = "gate-v1"

// RuleGate activates experts from event kind, affect and hard-state hints.
type RuleGate struct {
	Version string
}

// NewRuleGate returns the bundled deterministic gate.
func NewRuleGate() *RuleGate {
	return &RuleGate{Version: RULE_GATE_VERSION}
}

// Activate scores every expert and returns the top two or three. Ties break by
// DEFAULT_EXPERT_ORDER; an all-zero turn falls back to the default pair.
func (g *RuleGate) Activate(features Features) ([]string, error) {
	scores := map[string]float64{
		EXPERT_RELATION:   0,
		EXPERT_EVENT:      0,
		EXPERT_SELF:       0,
		EXPERT_KNOWLEDGE:  0,
		EXPERT_EXPRESSION: 1, // every answer needs phrasing
	}
	kind := strings.ToLower(strings.TrimSpace(features.EventKind))
	switch kind {
	case "relationship", "conflict", "apology":
		scores[EXPERT_RELATION] += 3
		scores[EXPERT_EXPRESSION] += 2
	case "question", "knowledge":
		scores[EXPERT_KNOWLEDGE] += 3
	case "emotion", "self":
		scores[EXPERT_SELF] += 3
	case "event", "turn":
		scores[EXPERT_EVENT] += 2
	}
	if strings.TrimSpace(features.Summary) != "" {
		scores[EXPERT_EVENT] += 1
	}
	if trust, ok := features.Affect["trust"]; ok && trust < 0.3 {
		scores[EXPERT_RELATION] += 2
	}
	if valence, ok := features.Affect["valence"]; ok {
		if valence > 0.6 || valence < -0.6 {
			scores[EXPERT_SELF] += 1
		}
	}
	for key := range features.HardState {
		if strings.Contains(key, EXPERT_KNOWLEDGE) {
			scores[EXPERT_KNOWLEDGE] += 1
			break
		}
	}
	return selectExperts(scores), nil
}

// FALLBACK_EXPERTS fills the minimum when scores leave too few experts; event
// memory and phrasing are the two that always make sense.
var FALLBACK_EXPERTS = []string{EXPERT_EVENT, EXPERT_EXPRESSION}

// selectExperts returns the highest scoring experts with stable tie-breaking,
// padded to the minimum count and ordered canonically.
func selectExperts(scores map[string]float64) []string {
	ordered := append([]string(nil), DEFAULT_EXPERT_ORDER...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return scores[ordered[i]] > scores[ordered[j]]
	})
	active := make([]string, 0, MAX_ACTIVE_EXPERTS)
	for _, name := range ordered {
		if len(active) == MAX_ACTIVE_EXPERTS {
			break
		}
		if scores[name] <= 0 {
			continue
		}
		active = append(active, name)
	}
	for _, name := range FALLBACK_EXPERTS {
		if len(active) >= MIN_ACTIVE_EXPERTS {
			break
		}
		if !contains(active, name) {
			active = append(active, name)
		}
	}
	// Canonical order keeps journal output stable across implementations.
	canonical := make([]string, 0, len(active))
	for _, name := range DEFAULT_EXPERT_ORDER {
		if contains(active, name) {
			canonical = append(canonical, name)
		}
	}
	return canonical
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
