// types.go declares the K4 settlement carriers: Completion, Report and Change.
// K4 freezes the Completion/Report field lists; Change is the report row this
// package adds so callers can audit exactly which source-layer writes happened.
package settle

import "github.com/RuiCat/Neko/internal/flow/spec"

// COMPLETION_VERSION is the K4 carrier version. Adding fields is compatible;
// removing or re-typing a frozen field is not.
const COMPLETION_VERSION = 1

// RESULT_COMPLETED is the result recorded on every Completion. K4 leaves the
// text to the producer; this value matches the task kernel's terminal result so
// m5 can route on one constant instead of parsing prose.
const RESULT_COMPLETED = "完成"

// Completion is the K4 completion event: one finished settlement of FlowID on
// Entity at Tick. Exactly one Completion exists per (FlowID, Entity).
type Completion struct {
	FlowID string `json:"flowID"`
	Entity string `json:"entity"`
	Result string `json:"result"`
	Tick   int    `json:"tick"`
}

// ChangeOp classifies one report row by what it represents.
type ChangeOp string

// Change operations. Spend rows carry the trade request; consume/produce rows
// target one material item; effect rows write one K1 effect.
const (
	// OP_SPEND is one trade-kernel request row for the flow cost.
	OP_SPEND ChangeOp = "spend"
	// OP_CONSUME removes quantity from a material item in the flow scene.
	OP_CONSUME ChangeOp = "consume"
	// OP_PRODUCE adds a template item to the flow scene.
	OP_PRODUCE ChangeOp = "produce"
	// OP_EFFECT writes one K1 effect to a source-layer field.
	OP_EFFECT ChangeOp = "effect"
)

// Change is one source-layer write planned by settlement. Entity is the actor
// for spend/effect rows and the item name for consume/produce rows; Scene is
// set only for material-item rows. Value is handed to Store.Apply verbatim,
// and Item carries the template shape when a produce row may create an entity.
type Change struct {
	Op     ChangeOp        `json:"op"`
	Entity string          `json:"entity"`
	Scene  string          `json:"scene,omitempty"`
	Path   string          `json:"path"`
	Kind   spec.EffectKind `json:"kind"`
	Value  any             `json:"value"`
	Item   *spec.ItemSpec  `json:"item,omitempty"`
}

// Report is the K4 settlement report. Changes lists the rows handed to the
// store (empty on refusal and on replay); Errors lists every refusal or
// failure and is non-nil on both success and failure.
type Report struct {
	Changes []Change `json:"changes"`
	Errors  []error  `json:"errors"`
}

// Failed reports whether settlement was refused. A report with no errors is a
// successful settlement, including an idempotent replay.
func (r Report) Failed() bool {
	return len(r.Errors) > 0
}
