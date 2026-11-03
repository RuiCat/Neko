// Package replay audits journal records against the snapshots they claim to
// derive from. It calls no model: it re-runs the package validator so a
// drifted or hand-edited record cannot pass unnoticed.
package replay

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/journal"
)

// Issue is one audit finding.
type Issue struct {
	TurnID  string `json:"turn_id"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Result summarises one audit pass.
type Result struct {
	Records          int     `json:"records"`
	WithCompensation int     `json:"with_compensation"`
	Valid            int     `json:"valid"`
	Invalid          int     `json:"invalid"`
	Issues           []Issue `json:"issues,omitempty"`
}

// Audit checks every conversation record with a compensation package. Engine
// tick snapshots and records without a package are counted but not judged.
func Audit(records []journal.TurnRecord) Result {
	result := Result{Records: len(records)}
	for _, record := range records {
		if record.Source == journal.SOURCE_TICK {
			continue
		}
		if record.Compensation.SnapshotID == "" {
			continue
		}
		result.WithCompensation++
		issues := auditRecord(record)
		if len(issues) == 0 {
			result.Valid++
			continue
		}
		result.Invalid++
		result.Issues = append(result.Issues, issues...)
	}
	return result
}

// auditRecord returns every finding for one record.
func auditRecord(record journal.TurnRecord) []Issue {
	issues := []Issue{}
	if record.TurnID == "" || record.TurnID != record.Snapshot.SnapshotID {
		issues = append(issues, Issue{TurnID: record.TurnID, Field: "turn_id", Message: "turn_id 与 snapshot_id 不一致"})
	}
	if err := compensation.Validate(record.Compensation, record.Snapshot); err != nil {
		issues = append(issues, Issue{TurnID: record.TurnID, Field: "compensation", Message: err.Error()})
	}
	if record.Gate.Version == gate.RULE_GATE_VERSION {
		if count := len(record.Gate.Active); count < 2 || count > 3 {
			issues = append(issues, Issue{
				TurnID: record.TurnID, Field: "gate.active",
				Message: fmt.Sprintf("激活专家数 %d 不在 2–3 之间", count),
			})
		}
	}
	return issues
}
