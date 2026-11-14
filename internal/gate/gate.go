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

// PHRASING_EXPERT 是每轮都应当有的「表达偏好」专家（规则门控给它最低分 1）。
// 模型门控始终把它放进激活集，因此训练标签只需要覆盖语义专家。
const PHRASING_EXPERT = EXPERT_EXPRESSION

// SEMANTIC_EXPERTS 是模型分类的语义专家集合（DEFAULT_EXPERT_ORDER 去掉表达偏好，
// 顺序即模型输出层类别顺序；改动需同步升模型版本）。
var SEMANTIC_EXPERTS = []string{EXPERT_RELATION, EXPERT_EVENT, EXPERT_SELF, EXPERT_KNOWLEDGE}

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

// Versioned is the optional interface a gate implements to report the version
// written into journal gate records (the bundled rule gate version otherwise).
type Versioned interface {
	Version() string
}

// PrimarySelector is the optional interface a gate implements to report the
// expert it considers primary for one turn, independent of the canonical
// activation order Activate returns. Training labels use it: a turn whose top
// expert is not first in canonical order still trains that expert.
type PrimarySelector interface {
	// PrimaryExpert returns the primary semantic expert for these features and
	// activation set, or "" when it cannot decide.
	PrimaryExpert(features Features, active []string) string
}

// VersionOf returns the gate's own version when it implements Versioned,
// falling back to RULE_GATE_VERSION so journal records stay honest across a
// rule-gate / model-gate swap.
func VersionOf(g Gate) string {
	if versioned, ok := g.(Versioned); ok {
		if version := versioned.Version(); version != "" {
			return version
		}
	}
	return RULE_GATE_VERSION
}
