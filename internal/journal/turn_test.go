// turn_test.go pins the join between a finished tick and a journal record.
package journal

import (
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/hardstate"
)

func TestRecordTurnCapturesSnapshot(t *testing.T) {
	cat := &entity.Entity{
		ID:   "猫猫",
		Type: "NPC",
		Data: map[string]any{"状态": map[string]any{"情绪": "平静"}},
	}
	now := time.Date(2026, 11, 1, 21, 0, 0, 0, time.Local)
	record, err := RecordTurn(TurnOptions{
		Entities:  []*entity.Entity{cat},
		Schema:    hardstate.DefaultSchema(),
		Sequence:  hardstate.NewSequence(),
		SessionID: "session-1",
		Now:       now,
	}, engine.TickResult{TickID: "tick-1"})
	if err != nil {
		t.Fatal(err)
	}
	if record.TurnID != "evt_20261101_0001" || record.Snapshot.SnapshotID != record.TurnID {
		t.Fatalf("轮次 ID 错误: %q / %q", record.TurnID, record.Snapshot.SnapshotID)
	}
	if record.Snapshot.TickID != "tick-1" {
		t.Fatalf("tick ID 未带上: %q", record.Snapshot.TickID)
	}
	if record.Status != STATUS_INCOMPLETE {
		t.Fatalf("状态 %q，期望 incomplete", record.Status)
	}
	if record.SessionID != "session-1" {
		t.Fatalf("会话 ID 未带上: %q", record.SessionID)
	}
	digest, err := hardstate.DigestOf(record.Snapshot.State)
	if err != nil {
		t.Fatal(err)
	}
	if record.Snapshot.Digest != digest {
		t.Fatalf("记录 digest 与状态不一致: %s vs %s", record.Snapshot.Digest, digest)
	}
}

func TestRecordTurnRequiresSequenceAndTime(t *testing.T) {
	if _, err := RecordTurn(TurnOptions{Now: time.Now()}, engine.TickResult{}); err == nil {
		t.Fatal("缺少序列应报错")
	}
	if _, err := RecordTurn(TurnOptions{Sequence: hardstate.NewSequence()}, engine.TickResult{}); err == nil {
		t.Fatal("缺少时间应报错")
	}
}
