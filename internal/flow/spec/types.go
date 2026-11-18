// types.go declares the frozen K1 wire contract shared by the P4 flow engine:
// FlowSpec (m2-m5 input), EffectSpec/EffectMap (C2 kind registry) and
// MigrationReport (C1 snapshot binding). JSON tags are the artifact format;
// schemaVersion is 1 and may only grow by adding fields.
package spec

// SCHEMA_VERSION is the current K1 artifact version. The contract allows new
// fields to be added, never removed or re-typed.
const SCHEMA_VERSION = 1

// Source anchors a datum to a corpus file and a 1-based line number. The file
// is the absolute corpus path so the C1 judgment script can reopen it directly.
type Source struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// ItemSpec is one item produced by a flow. The three fields map to the corpus
// keys 类型/名称/描述; an empty description is allowed.
type ItemSpec struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FlowSpec is one executable legacy template. Requires are items that must be
// present, Consume are items removed on settlement, and Cost is the currency
// amount charged. Effects stay ordered exactly as written in the corpus.
type FlowSpec struct {
	ID       string       `json:"id"`
	Source   Source       `json:"source"`
	Scene    string       `json:"scene"`
	Requires []string     `json:"requires"`
	Cost     int          `json:"cost"`
	Consume  []string     `json:"consume"`
	Produce  []ItemSpec   `json:"produce"`
	Steps    []string     `json:"steps"`
	Effects  []EffectSpec `json:"effects"`
}

// TaskCondition is the normalized 条件 object: type is the corpus 类型/string
// and example is the corpus 示例 (empty when the source only names the type).
type TaskCondition struct {
	Type    string `json:"type"`
	Example string `json:"example"`
}

// TaskReward is the normalized 奖励 object. The corpus stores 经验/好感度 as an
// object in the library and as a Chinese string in cat_quest; both map here.
type TaskReward struct {
	Experience int `json:"experience"`
	Affinity   int `json:"affinity"`
}

// TaskDialog is the normalized 对话 object. Fields are empty when the source
// template has no 对话 block (cat_quest's five partial templates); they are not
// guessed from other templates.
type TaskDialog struct {
	Activate   string `json:"activate"`
	InProgress string `json:"inProgress"`
	Complete   string `json:"complete"`
}

// TaskTemplateSpec is one normalized task template. Stages use the task
// contract's 未激活 (the corpus 已激活 is mapped at extraction time per C4).
type TaskTemplateSpec struct {
	ID        string        `json:"id"`
	Source    Source        `json:"source"`
	Stages    []string      `json:"stages"`
	Condition TaskCondition `json:"condition"`
	Reward    TaskReward    `json:"reward"`
	Dialog    TaskDialog    `json:"dialog"`
}

// EffectKind is the closed C2 classification set. A numeric literal is set, a
// signed +/- literal is add, a quoted literal is text and true/false is bool.
type EffectKind string

const (
	EFFECT_KIND_SET  EffectKind = "set"
	EFFECT_KIND_ADD  EffectKind = "add"
	EFFECT_KIND_TEXT EffectKind = "text"
	EFFECT_KIND_BOOL EffectKind = "bool"
)

// EffectSpec is one effect occurrence. Value is a json.Number for set/add, a
// string for text and a bool for bool; Source points at the effect key line.
// Target is the settlement source path (nil when no mapping exists) and
// TargetReason records the mapping basis or the reason a null is intentional.
type EffectSpec struct {
	Path         string     `json:"path"`
	Kind         EffectKind `json:"kind"`
	Value        any        `json:"value"`
	Source       Source     `json:"source"`
	Target       *string    `json:"target"`
	TargetReason string     `json:"targetReason"`
}

// UnclassifiedEffect records an effect whose literal is outside the C2 kinds.
// It is counted and listed instead of being guessed into a kind.
type UnclassifiedEffect struct {
	Path   string `json:"path"`
	Source Source `json:"source"`
	Raw    string `json:"raw"`
	Reason string `json:"reason"`
}

// EffectMap is the C2 artifact. Unclassified is a count (the C2 judgment script
// compares it to zero); UnclassifiedItems carries the detail behind that count.
type EffectMap struct {
	SchemaVersion     int                  `json:"schemaVersion"`
	Effects           []EffectSpec         `json:"effects"`
	Unclassified      int                  `json:"unclassified"`
	UnclassifiedItems []UnclassifiedEffect `json:"unclassifiedItems"`
}

// FlowSpecFile is the flow-spec.json root. Flows and taskTemplates are ordered
// by corpus order, then definition order, then template order.
type FlowSpecFile struct {
	SchemaVersion int                `json:"schemaVersion"`
	Flows         []FlowSpec         `json:"flows"`
	TaskTemplates []TaskTemplateSpec `json:"taskTemplates"`
}

// MigrationRecord is one migrated unit with its reopened file:line anchor.
// Kind is additive metadata: flow, r18Flow or taskTemplate.
type MigrationRecord struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

// Container is one @定义 block. It is reported separately from leaf records so
// structural blocks are accounted for without inflating the record count.
type Container struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Name string `json:"name"`
}

// SkippedBlock is one parsed-but-not-migrated block (legacy @规则, NPC schema,
// unknown definition). Skipping is explicit so nothing disappears silently.
type SkippedBlock struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Directive string `json:"directive"`
	Reason    string `json:"reason"`
}

// ParseIssue is one parser complaint. Issues are counted in the report instead
// of aborting the whole extraction.
type ParseIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// FileHash binds one corpus file to its sha256.
type FileHash struct {
	File   string `json:"file"`
	Sha256 string `json:"sha256"`
}

// MigrationReport is the C1 artifact. SnapshotSha256 is the sha256 of the five
// corpus files concatenated in corpus order; SnapshotSha256Compat is the exact
// GNU sha256sum manifest text expected by the C1 judgment field name
// snapshot_sha256; SnapshotFiles carries the per-file hashes.
type MigrationReport struct {
	SchemaVersion        int               `json:"schemaVersion"`
	SnapshotSha256       string            `json:"snapshotSha256"`
	SnapshotSha256Compat string            `json:"snapshot_sha256"`
	SnapshotFiles        []FileHash        `json:"snapshotFiles"`
	Count                int               `json:"count"`
	Records              []MigrationRecord `json:"records"`
	ContainerCount       int               `json:"containerCount"`
	Containers           []Container       `json:"containers"`
	SkippedCount         int               `json:"skippedCount"`
	Skipped              []SkippedBlock    `json:"skipped"`
	ErrorCount           int               `json:"errorCount"`
	Errors               []ParseIssue      `json:"errors"`
}

// Artifacts groups the three JSON artifacts produced by one generation run.
type Artifacts struct {
	Flows   FlowSpecFile
	Effects EffectMap
	Report  MigrationReport
}
