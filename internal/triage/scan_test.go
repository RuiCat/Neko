// triage_test.go pins the A/B scan and the class routing that decides what the
// repair queue is allowed to see.
package triage

import (
	"testing"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

func baseRecord() journal.TurnRecord {
	snapshot := hardstate.Snapshot{SnapshotID: "evt_20261103_0001", TurnID: "evt_20261103_0001"}
	snapshot.State = map[string]any{"猫猫.状态.情绪": "平静"}
	return journal.TurnRecord{
		TurnID: "evt_20261103_0001", Source: journal.SOURCE_DSH_TURN,
		Snapshot: snapshot,
		Compensation: journal.Compensation{
			SnapshotID: "evt_20261103_0001", StateSource: "state_machine_snapshot",
			State: map[string]any{"猫猫.状态.情绪": "平静"},
		},
		Gate: journal.Gate{Version: "gate-v1", Active: []string{"事件记忆", "表达偏好"}},
	}
}

func TestScanCleanRecordHasNoFindings(t *testing.T) {
	if findings := Scan([]journal.TurnRecord{baseRecord()}, nil); len(findings) != 0 {
		t.Fatalf("干净记录不应有 finding: %+v", findings)
	}
}

func TestScanMissingSnapshot(t *testing.T) {
	record := baseRecord()
	record.Snapshot = hardstate.Snapshot{}
	findings := Scan([]journal.TurnRecord{record}, nil)
	if len(findings) != 1 || findings[0].Kind != "missing_snapshot" || findings[0].Class != ClassConsistency {
		t.Fatalf("缺少快照应报 consistency: %+v", findings)
	}
}

func TestScanTurnIDMismatch(t *testing.T) {
	record := baseRecord()
	record.Snapshot.SnapshotID = "evt_other"
	findings := Scan([]journal.TurnRecord{record}, nil)
	if !containsKind(findings, "turn_id_mismatch") {
		t.Fatalf("应报 turn_id_mismatch: %+v", findings)
	}
}

func TestScanCompensationInvalid(t *testing.T) {
	record := baseRecord()
	record.Compensation.State = map[string]any{"猫猫.状态.情绪": "愤怒"}
	findings := Scan([]journal.TurnRecord{record}, nil)
	if !containsKind(findings, "compensation_invalid") {
		t.Fatalf("状态与快照不一致应报 compensation_invalid: %+v", findings)
	}
}

func TestScanGateCount(t *testing.T) {
	record := baseRecord()
	record.Gate.Active = []string{"事件记忆"}
	findings := Scan([]journal.TurnRecord{record}, nil)
	if !containsKind(findings, "gate_count") {
		t.Fatalf("专家数异常应报 gate_count: %+v", findings)
	}
}

func TestScanAnnotatedIncorrectIsFact(t *testing.T) {
	record := baseRecord()
	wrong := false
	labels := map[string][]annotations.Record{
		record.TurnID: {{TurnID: record.TurnID, Correct: &wrong, Annotator: "a"}},
	}
	findings := Scan([]journal.TurnRecord{record}, labels)
	if !containsClass(findings, ClassFact) {
		t.Fatalf("判错应归 fact: %+v", findings)
	}
}

func TestScanUnjudgedIsPreferenceNotFact(t *testing.T) {
	record := baseRecord()
	labels := map[string][]annotations.Record{
		record.TurnID: {{TurnID: record.TurnID, Annotator: "a"}},
	}
	findings := Scan([]journal.TurnRecord{record}, labels)
	if containsClass(findings, ClassFact) {
		t.Fatalf("未判定不应归 fact: %+v", findings)
	}
	if !containsClass(findings, ClassPreference) {
		t.Fatalf("未判定应归 preference: %+v", findings)
	}
}

func TestScanSkipsTickRecords(t *testing.T) {
	record := baseRecord()
	record.Source = journal.SOURCE_TICK
	record.Snapshot = hardstate.Snapshot{}
	if findings := Scan([]journal.TurnRecord{record}, nil); len(findings) != 0 {
		t.Fatalf("tick 记录应被跳过: %+v", findings)
	}
}

func containsKind(findings []Finding, kind string) bool {
	for _, finding := range findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func containsClass(findings []Finding, class Class) bool {
	for _, finding := range findings {
		if finding.Class == class {
			return true
		}
	}
	return false
}
