// result.go holds the types one tick reports back to the caller: the tick
// outcome, per-entity transitions, and every state write observed while it ran.
package engine

// TickResult is one tick's outcome. JSON tags use snake_case to match Python.
type TickResult struct {
	TickID         string       `json:"tick_id"`
	Status         string       `json:"status"`
	Transitions    []Transition `json:"transitions"`
	Conflicts      []Conflict   `json:"conflicts"`
	ReplyHint      string       `json:"hint"`
	RulesEvaluated int          `json:"rules_evaluated"`
	Errors         []string     `json:"errors"`
	// Writes 是本 tick 内观察到的**所有**实体写入（规则 op、文本效果/赋值副作用、
	// @函数/事件里的隐式写入），带写入前后的值；事件黑板据此把「规则之外的写入」
	// 也变成可推送的变化。
	Writes []EntityWrite `json:"writes,omitempty"`
}

// EntityWrite is one state write observed during a tick. RuleID is the rule
// being applied when the write happened ("" for writes outside rule actions,
// for example @函数 during condition evaluation).
type EntityWrite struct {
	EntityID string `json:"entity_id"`
	RuleID   string `json:"rule_id,omitempty"`
	Path     string `json:"path"`
	From     any    `json:"from"`
	To       any    `json:"to"`
}
