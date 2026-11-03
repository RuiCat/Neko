// Package gate decides which experts run for a turn. The interface is frozen
// (C5); the bundled rule gate is a deterministic v1 that can be swapped for a
// trained model without changing callers.
package gate

// Expert IDs used by the gate and by journal gate records. They are the same
// strings the compensation assembler will use, so logs stay joinable.
const (
	EXPERT_RELATION   = "关系记忆"
	EXPERT_EVENT      = "事件记忆"
	EXPERT_SELF       = "自我记忆"
	EXPERT_KNOWLEDGE  = "知识边界"
	EXPERT_EXPRESSION = "表达偏好"
)

// DEFAULT_EXPERT_ORDER is the tie-break and fallback order.
var DEFAULT_EXPERT_ORDER = []string{
	EXPERT_RELATION, EXPERT_EVENT, EXPERT_SELF, EXPERT_KNOWLEDGE, EXPERT_EXPRESSION,
}

// Features is the gate input: structured event kind, the hard-state snapshot
// values, an affect vector and a short dialogue summary. No raw text history.
type Features struct {
	EventKind string
	HardState map[string]any
	Affect    map[string]float64
	Summary   string
}

// Gate activates 2–3 experts for one turn. Implementations must be
// deterministic for identical features.
type Gate interface {
	Activate(features Features) ([]string, error)
}
