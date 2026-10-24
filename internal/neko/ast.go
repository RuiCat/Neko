// Package neko implements the .neko script dialect: block scanning, parsing and
// the AST shared by the compiler.
package neko

// Pos is a 1-based source position.
type Pos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Block is one top-level block produced by the lexer.
type Block struct {
	Kind string `json:"kind"` // at_data | at_data_legacy | at_rule | at_event | at_func | hash_rule | meta
	Name string `json:"name"`
	Body string `json:"body"`
	Line int    `json:"line"`
}

// Pos returns the block position.
func (b Block) Pos() Pos { return Pos{Line: b.Line} }

// DataDef is an @Data entity template definition (or an @data non-entity block).
type DataDef struct {
	TypeName string         `json:"type_name"`
	Data     map[string]any `json:"data"`
	IsEntity bool           `json:"is_entity"`
	Pos      Pos            `json:"pos"`
}

// Subject is one rule subject such as $1[NPC] or 场景.
type Subject struct {
	Var       string `json:"var"`       // "", "$", "$1", "$2"
	Kind      string `json:"kind"`      // primary | multi | global
	TypeName  string `json:"type_name"` // NPC
	Condition string `json:"condition"` // 性别=女
	EntityID  string `json:"entity_id"` // global id (场景/世界/...)
}

// ActionOp is one parsed rule action.
type ActionOp struct {
	Op      string `json:"op"` // set | add | append | emit
	Target  string `json:"target"`
	Value   any    `json:"value"`
	Subject int    `json:"subject,omitempty"` // 1 -> $1
	Entity  string `json:"entity,omitempty"`
	Pos     Pos    `json:"pos"`
}

// BranchDef is one parsed state-machine branch.
type BranchDef struct {
	Condition *string        `json:"condition,omitempty"` // nil means else/default
	Ops       []ActionOp     `json:"ops"`
	State     map[string]any `json:"state,omitempty"`
	Pos       Pos            `json:"pos"`
}

// RuleDef is a parsed rule block or hash rule line.
type RuleDef struct {
	Subjects   []Subject   `json:"subjects"`
	HeaderRaw  string      `json:"header_raw"`
	RuleID     string      `json:"rule_id,omitempty"`
	StateWhen  *string     `json:"state_when,omitempty"`
	Conditions []string    `json:"conditions"`
	Actions    []ActionOp  `json:"actions"`
	Branches   []BranchDef `json:"branches"`
	Priority   *int        `json:"priority,omitempty"`
	LineForm   *string     `json:"line_form,omitempty"`
	Pos        Pos         `json:"pos"`
}

// MetaDef is a metadata line such as #世界观: ...
type MetaDef struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Pos   Pos    `json:"pos"`
}

// EventDef is an @事件 definition.
type EventDef struct {
	Name      string `json:"name"`
	Condition string `json:"condition,omitempty"`
	Handler   string `json:"handler,omitempty"`
	Actions   []any  `json:"actions,omitempty"`
	Pos       Pos    `json:"pos"`
}

// Param is one function parameter.
type Param struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// FuncDef is an @func definition (body is .neko source).
type FuncDef struct {
	Name       string  `json:"name"`
	Params     []Param `json:"params"`
	ReturnType string  `json:"return_type,omitempty"`
	Body       string  `json:"body"`
	Pos        Pos     `json:"pos"`
}

// ScriptAST is the complete parse result.
type ScriptAST struct {
	DataDefs  []DataDef  `json:"data_defs"`
	RuleDefs  []RuleDef  `json:"rule_defs"`
	EventDefs []EventDef `json:"event_defs"`
	FuncDefs  []FuncDef  `json:"func_defs"`
	Metadata  []MetaDef  `json:"metadata"`
	Errors    []error    `json:"-"`
	Warnings  []Warning  `json:"warnings"`
}

// Warning is a non-fatal parse or compile note.
type Warning struct {
	Message string `json:"message"`
	Pos     Pos    `json:"pos"`
}
