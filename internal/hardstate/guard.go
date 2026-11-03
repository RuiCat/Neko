// guard.go classifies writes against the hard-state schema. The state machine
// is the only legal writer; other sources get a violation verdict so the
// engine can warn first and block later (D2 rollout is advisory in P2).
package hardstate

import "fmt"

// Write sources, used in violation messages and future blocking decisions.
const (
	SOURCE_STATE_MACHINE = "state_machine"
	SOURCE_SCRIPT        = "script"
	SOURCE_MCP           = "mcp"
	SOURCE_CONTROL       = "control"
	SOURCE_UNKNOWN       = "unknown"
)

// WriteCheck is the verdict for one attempted write. HardState is false when
// the write does not touch the hard state at all; Allowed is true for legal
// writers and for non-hard-state paths.
type WriteCheck struct {
	HardState bool
	Allowed   bool
	Reason    string
}

// CheckWrite reports whether source may write entityID.path under schema. A
// schema that is not configured disables the guard (zero WriteCheck) so
// callers without wiring behave exactly as before.
func CheckWrite(schema Schema, entityID, path, source string) WriteCheck {
	if schema.Version < 1 || schema.Group == "" {
		return WriteCheck{}
	}
	if !schema.includes(entityID) {
		return WriteCheck{}
	}
	if !schema.Covers(path) {
		return WriteCheck{}
	}
	if source == SOURCE_STATE_MACHINE {
		return WriteCheck{HardState: true, Allowed: true}
	}
	if source == "" {
		source = SOURCE_UNKNOWN
	}
	return WriteCheck{
		HardState: true,
		Allowed:   false,
		Reason:    fmt.Sprintf("硬状态字段 %s.%s 只能由状态机写入（来源 %s）", entityID, path, source),
	}
}
