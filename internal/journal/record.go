// record.go defines the per-turn interaction record (C6): the single on-disk
// shape every metric and every nightly scan reads. One line per turn.
package journal

import (
	"time"

	"github.com/RuiCat/Neko/internal/hardstate"
)

// RECORD_VERSION is bumped when the on-disk record shape changes.
const RECORD_VERSION = 1

// Turn statuses. Incomplete means the record has a snapshot but no answer yet
// (the P1 wiring writes these); complete means a model answered.
const (
	STATUS_INCOMPLETE = "incomplete"
	STATUS_COMPLETE   = "complete"
	STATUS_ERROR      = "error"
)

// Record sources distinguish a conversation turn from an engine tick. Metrics
// and replay skip SOURCE_TICK so periodic state snapshots are not counted as
// user turns.
const (
	SOURCE_TICK     = "tick"
	SOURCE_DSH_TURN = "dsh-turn"
	SOURCE_CHAT     = "chat"
)

// TurnRecord is one turn of the interaction loop. It intentionally carries no
// state numbers of its own: the snapshot is the only hard-state source, and
// Compensation.State must be a projection of that snapshot.
type TurnRecord struct {
	RecordVersion int                `json:"record_version"`
	TurnID        string             `json:"turn_id"`
	SessionID     string             `json:"session_id,omitempty"`
	Status        string             `json:"status"`
	Source        string             `json:"source,omitempty"`
	StartedAt     time.Time          `json:"started_at"`
	EndedAt       *time.Time         `json:"ended_at,omitempty"`
	UserInput     string             `json:"user_input"`
	Snapshot      hardstate.Snapshot `json:"snapshot"`
	Gate          Gate               `json:"gate"`
	Compensation  Compensation       `json:"compensation"`
	Answer        string             `json:"answer"`
	AnswerModel   string             `json:"answer_model,omitempty"`
	PromptVersion string             `json:"prompt_version,omitempty"`
	Cites         []Citation         `json:"cites,omitempty"`
	Feedback      Feedback           `json:"feedback"`
	LatencyMS     int64              `json:"latency_ms"`
	Cost          Cost               `json:"cost"`
}

// Gate records which experts were activated for this turn.
type Gate struct {
	Active  []string `json:"active,omitempty"`
	Version string   `json:"version,omitempty"`
}

// Compensation mirrors the assembler output (C4). State keys are entity
// qualified ("猫猫.状态.情绪") so they resolve against the snapshot directly.
type Compensation struct {
	Memory             []MemoryFragment `json:"memory,omitempty"`
	State              map[string]any   `json:"state,omitempty"`
	StateSource        string           `json:"state_source,omitempty"`
	PersonaConstraints []string         `json:"persona_constraints,omitempty"`
	Situation          string           `json:"situation,omitempty"`
	TurnID             string           `json:"turn_id,omitempty"`
	SnapshotID         string           `json:"snapshot_id,omitempty"`
}

// MemoryFragment is one retrieved or reconstructed memory item. Type is
// "retrieved" or "reconstructed"; reconstructed items carry a lower weight.
type MemoryFragment struct {
	Content          string  `json:"content"`
	SourceSnapshotID string  `json:"source_snapshot_id"`
	Type             string  `json:"type"`
	Weight           float64 `json:"weight"`
}

// Citation is one past event the answer claims to use.
type Citation struct {
	EventID string `json:"event_id"`
	Kind    string `json:"kind"`
}

// Feedback is the user's explicit signal; "none" means no signal.
type Feedback struct {
	Kind string `json:"kind"`
	Note string `json:"note,omitempty"`
}

// Cost records token usage for budget accounting.
type Cost struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
