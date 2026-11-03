// tickJournal_test.go proves one tick yields one journal record with the
// session and tick identifiers attached.
package mcpserver

import (
	"testing"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

func TestRunTickWritesJournalRecord(t *testing.T) {
	fixture := newTickFixture(t)
	if err := fixture.entity.Set("状态.情绪", "平静"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	dir := t.TempDir()
	writer := journal.NewWriter(dir)
	fixture.server.SetJournal(writer, hardstate.NewSequence(), hardstate.DefaultSchema())
	fixture.server.SetSessionID("session-42")
	fixture.server.SetLearningLoop(
		blackboard.New(),
		compensation.NewAssembler(gate.NewRuleGate(), expert.NewDefaultRegistry(4)),
	)

	result, err := fixture.server.RunTick("", TickOptions{SkipSaveWhenUnchanged: true})
	if err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	records, err := journal.ReadDir(dir)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("记录数 %d，期望 1", len(records))
	}
	record := records[0]
	if record.SessionID != "session-42" {
		t.Fatalf("会话 ID %q", record.SessionID)
	}
	if record.Snapshot.TickID != result.TickID {
		t.Fatalf("快照 tick ID %q ≠ %q", record.Snapshot.TickID, result.TickID)
	}
	if record.TurnID == "" || record.Snapshot.SnapshotID != record.TurnID {
		t.Fatalf("轮次 ID 未生成: %+v", record)
	}
	if record.Status != journal.STATUS_INCOMPLETE {
		t.Fatalf("状态 %q", record.Status)
	}
	if record.Compensation.StateSource != compensation.STATE_SOURCE {
		t.Fatalf("补偿包来源 %q", record.Compensation.StateSource)
	}
	if record.Compensation.State["猫猫.状态.情绪"] != "平静" {
		t.Fatalf("补偿状态未从快照投影：%+v", record.Compensation.State)
	}
	if len(record.Gate.Active) < 2 || len(record.Gate.Active) > 3 {
		t.Fatalf("激活专家数 %d：%v", len(record.Gate.Active), record.Gate.Active)
	}
	if record.Gate.Version != gate.RULE_GATE_VERSION {
		t.Fatalf("门控版本 %q", record.Gate.Version)
	}
}
