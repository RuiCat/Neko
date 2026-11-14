// event.go defines one observation event, its tier vocabulary and the scope
// naming helpers shared by the bus, the policy and the control plane.
package observation

import "strings"

// Tier says how much an event is worth pushing: critical is never dropped by
// the budget, normal obeys it, noise stays panel-only and never reaches a push.
type Tier string

const (
	// TierCritical is a state word, death, pregnancy or shock: always pushed.
	TierCritical Tier = "critical"
	// TierNormal is an ordinary change: pushed when the budget allows it.
	TierNormal Tier = "normal"
	// TierNoise is a high-frequency numeric field: panel-only.
	TierNoise Tier = "noise"
)

// rank orders tiers for rendering (smaller first) and for merging duplicates.
func (t Tier) rank() int {
	switch t {
	case TierCritical:
		return 0
	case TierNormal:
		return 1
	default:
		return 2
	}
}

// Event is one append-only state movement inside a scope. Seq is assigned by
// the bus and never reused; At is UTC RFC3339. Tier is filled at read time by
// the policy, never stored.
type Event struct {
	Scope   string   `json:"scope"`
	Seq     int64    `json:"seq"`
	Path    string   `json:"path"`
	From    any      `json:"from"`
	To      any      `json:"to"`
	RuleIDs []string `json:"rule_ids,omitempty"`
	TickID  string   `json:"tick_id,omitempty"`
	At      string   `json:"at"`
	Tier    Tier     `json:"tier,omitempty"`
	// Seeded marks a baseline value written at startup so a state projection
	// can show long-lived fields. Seeded events never reach a delta push.
	Seeded bool `json:"seeded,omitempty"`
}

// ScopeForEntity returns the stream scope of one entity.
func ScopeForEntity(id string) string { return "entity:" + id }

// EntityOfScope returns the entity id of an entity scope, or "" for any other
// scope kind.
func EntityOfScope(scope string) string {
	return strings.TrimPrefix(scope, "entity:")
}

// IsEntityScope reports whether a scope addresses one entity.
func IsEntityScope(scope string) bool {
	return strings.HasPrefix(scope, "entity:") && len(scope) > len("entity:")
}
