// scan.go is the A-class full scan plus the B-class annotated sample. A-class
// findings are deterministic structure checks that need no model; B-class
// findings come from human labels and are never auto-repaired when subjective.
package triage

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/journal"
)

// Severity values carried by a finding.
const (
	SEVERITY_ERROR   = "error"
	SEVERITY_WARNING = "warning"
)

// Scan runs both passes. Engine tick snapshots are skipped: they are periodic
// state captures, not user turns.
func Scan(records []journal.TurnRecord, labels map[string][]annotations.Record) []Finding {
	findings := []Finding{}
	for _, record := range records {
		if record.Source == journal.SOURCE_TICK {
			continue
		}
		findings = append(findings, scanStructure(record)...)
		findings = append(findings, scanAnnotations(record, labels[record.TurnID])...)
	}
	return findings
}

// scanStructure is the A-class full scan: turn/snapshot identity, package
// validity and gate cardinality.
func scanStructure(record journal.TurnRecord) []Finding {
	findings := []Finding{}
	if record.Snapshot.SnapshotID == "" {
		findings = append(findings, Finding{
			TurnID: record.TurnID, Class: ClassConsistency, Kind: "missing_snapshot",
			Severity: SEVERITY_ERROR, Detail: "记录没有硬状态快照", Source: "A",
		})
		return findings
	}
	if record.TurnID != record.Snapshot.SnapshotID {
		findings = append(findings, Finding{
			TurnID: record.TurnID, SnapshotID: record.Snapshot.SnapshotID,
			Class: ClassConsistency, Kind: "turn_id_mismatch", Severity: SEVERITY_ERROR,
			Detail: fmt.Sprintf("turn_id %q 与 snapshot_id %q 不一致", record.TurnID, record.Snapshot.SnapshotID),
			Source: "A",
		})
	}
	if record.Compensation.SnapshotID != "" {
		if err := compensation.Validate(record.Compensation, record.Snapshot); err != nil {
			findings = append(findings, Finding{
				TurnID: record.TurnID, SnapshotID: record.Snapshot.SnapshotID,
				Class: ClassConsistency, Kind: "compensation_invalid", Severity: SEVERITY_ERROR,
				Detail: err.Error(), Source: "A",
			})
		}
	}
	if record.Gate.Version == gate.RULE_GATE_VERSION {
		if count := len(record.Gate.Active); count < 2 || count > 3 {
			findings = append(findings, Finding{
				TurnID: record.TurnID, SnapshotID: record.Snapshot.SnapshotID,
				Class: ClassConsistency, Kind: "gate_count", Severity: SEVERITY_WARNING,
				Detail: fmt.Sprintf("激活专家数 %d 不在 2–3 之间", count), Source: "A",
			})
		}
	}
	return findings
}

// scanAnnotations is the B-class sample. An explicitly wrong answer is a fact
// finding; an unjudged or subjective label is a preference finding and never
// becomes repair input.
func scanAnnotations(record journal.TurnRecord, labels []annotations.Record) []Finding {
	if len(labels) == 0 {
		return nil
	}
	var judged *bool
	for _, label := range labels {
		if label.Correct == nil {
			continue
		}
		judged = label.Correct
		if !*label.Correct {
			return []Finding{{
				TurnID: record.TurnID, SnapshotID: record.Snapshot.SnapshotID,
				Class: ClassFact, Kind: "answer_incorrect", Severity: SEVERITY_ERROR,
				Detail: "人工标注判定答案不正确", Source: "B",
			}}
		}
	}
	if judged == nil {
		return []Finding{{
			TurnID: record.TurnID, SnapshotID: record.Snapshot.SnapshotID,
			Class: ClassPreference, Kind: "unjudged_sample", Severity: SEVERITY_WARNING,
			Detail: "标注未判定正确性，按偏好样本处理（不进修复队列）", Source: "B",
		}}
	}
	return nil
}
