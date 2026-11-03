// writer_test.go pins the journal round-trip, ordering and sequence recovery.
package journal

import (
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/hardstate"
)

func testRecord(turnID string, startedAt time.Time) TurnRecord {
	return TurnRecord{
		RecordVersion: RECORD_VERSION,
		TurnID:        turnID,
		Status:        STATUS_INCOMPLETE,
		StartedAt:     startedAt,
		Feedback:      Feedback{Kind: "none"},
		Snapshot: hardstate.Snapshot{
			SnapshotID: turnID,
			TurnID:     turnID,
			TakenAt:    startedAt,
			Digest:     "sha256:abc",
			State:      map[string]any{"猫猫.状态.情绪": "平静"},
		},
	}
}

func TestWriterRoundTripAndOrder(t *testing.T) {
	dir := t.TempDir()
	writer := NewWriter(dir)
	day := time.Date(2026, 11, 1, 9, 0, 0, 0, time.Local)
	if err := writer.Append(testRecord("evt_20261101_0002", day.Add(2*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(testRecord("evt_20261101_0001", day)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("记录数 %d，期望 2", len(records))
	}
	if records[0].TurnID != "evt_20261101_0001" || records[1].TurnID != "evt_20261101_0002" {
		t.Fatalf("按开始时间排序错误: %s, %s", records[0].TurnID, records[1].TurnID)
	}
	if records[0].Snapshot.State["猫猫.状态.情绪"] != "平静" {
		t.Fatal("快照状态未往返")
	}
}

func TestMaxSequenceAndRestart(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 11, 1, 20, 0, 0, 0, time.Local)
	writer := NewWriter(dir)
	for index, id := range []string{"evt_20261101_0003", "evt_20261101_0007"} {
		if err := writer.Append(testRecord(id, day.Add(time.Duration(index)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	maxSeq, err := MaxSequence(dir, day)
	if err != nil {
		t.Fatal(err)
	}
	if maxSeq != 7 {
		t.Fatalf("最大序号 %d，期望 7", maxSeq)
	}
	sequence := hardstate.NewSequence()
	sequence.SetLast(maxSeq)
	if got := sequence.Next(day); got != "evt_20261101_0008" {
		t.Fatalf("重启后 ID %q，期望 evt_20261101_0008", got)
	}
}

func TestMaxSequenceMissingFileIsZero(t *testing.T) {
	maxSeq, err := MaxSequence(t.TempDir(), time.Now())
	if err != nil || maxSeq != 0 {
		t.Fatalf("空目录应返回 0,nil；得到 %d,%v", maxSeq, err)
	}
}

func TestAppendAfterCloseFails(t *testing.T) {
	writer := NewWriter(t.TempDir())
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(testRecord("evt_20261101_0001", time.Now())); err != ErrClosed {
		t.Fatalf("关闭后追加应返回 ErrClosed，得到 %v", err)
	}
}
