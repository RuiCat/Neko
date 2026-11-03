// replay_test.go pins the audit: a valid package passes, tampering is caught,
// and records without compensation are counted but not judged.
package replay

import (
	"testing"

	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

func validRecord() journal.TurnRecord {
	return journal.TurnRecord{
		TurnID: "evt_20261103_0001",
		Snapshot: hardstate.Snapshot{
			SnapshotID: "evt_20261103_0001",
			TurnID:     "evt_20261103_0001",
			State:      map[string]any{"猫猫.状态.情绪": "平静"},
		},
		Compensation: journal.Compensation{
			SnapshotID:  "evt_20261103_0001",
			TurnID:      "evt_20261103_0001",
			StateSource: compensation.STATE_SOURCE,
			State:       map[string]any{"猫猫.状态.情绪": "平静"},
		},
		Gate: journal.Gate{Active: []string{gate.EXPERT_EVENT, gate.EXPERT_EXPRESSION}, Version: gate.RULE_GATE_VERSION},
	}
}

func TestAuditValidRecord(t *testing.T) {
	result := Audit([]journal.TurnRecord{validRecord()})
	if result.WithCompensation != 1 || result.Valid != 1 || result.Invalid != 0 {
		t.Fatalf("审计结果错误：%+v", result)
	}
}

func TestAuditCatchesTamperedState(t *testing.T) {
	record := validRecord()
	record.Compensation.State["猫猫.状态.情绪"] = "暴怒"
	result := Audit([]journal.TurnRecord{record})
	if result.Invalid != 1 || len(result.Issues) == 0 {
		t.Fatalf("篡改状态未被发现：%+v", result)
	}
}

func TestAuditCatchesBadGateCount(t *testing.T) {
	record := validRecord()
	record.Gate.Active = []string{gate.EXPERT_EVENT}
	result := Audit([]journal.TurnRecord{record})
	if result.Invalid != 1 {
		t.Fatalf("专家数异常未被发现：%+v", result)
	}
}

func TestAuditCountsRecordsWithoutCompensation(t *testing.T) {
	record := journal.TurnRecord{TurnID: "evt_20261101_0001"}
	result := Audit([]journal.TurnRecord{record, validRecord()})
	if result.Records != 2 || result.WithCompensation != 1 || result.Valid != 1 {
		t.Fatalf("计数错误：%+v", result)
	}
}

func TestAuditSkipsTickRecords(t *testing.T) {
	tick := validRecord()
	tick.Source = journal.SOURCE_TICK
	result := Audit([]journal.TurnRecord{tick})
	if result.WithCompensation != 0 {
		t.Fatalf("tick 记录不应参与回放审计：%+v", result)
	}
}
