// sequence_test.go pins the snapshot ID format and the restart contract.
package hardstate

import (
	"testing"
	"time"
)

func TestSequenceFormatAndMonotonic(t *testing.T) {
	sequence := NewSequence()
	day := time.Date(2026, 11, 1, 23, 0, 0, 0, time.Local)
	first := sequence.Next(day)
	second := sequence.Next(day)
	if first != "evt_20261101_0001" || second != "evt_20261101_0002" {
		t.Fatalf("ID 格式错误: %q, %q", first, second)
	}
}

func TestSequenceResetsOnNewDay(t *testing.T) {
	sequence := NewSequence()
	firstDay := time.Date(2026, 11, 1, 23, 59, 0, 0, time.Local)
	nextDay := time.Date(2026, 11, 2, 0, 1, 0, 0, time.Local)
	sequence.Next(firstDay)
	sequence.Next(firstDay)
	if got := sequence.Next(nextDay); got != "evt_20261102_0001" {
		t.Fatalf("跨日应重置序号，得到 %q", got)
	}
}

func TestSequenceSetLastSeedsFirstID(t *testing.T) {
	sequence := NewSequence()
	sequence.SetLast(41)
	got := sequence.Next(time.Date(2026, 11, 1, 8, 0, 0, 0, time.Local))
	if got != "evt_20261101_0042" {
		t.Fatalf("重启后应从 seed+1 开始，得到 %q", got)
	}
}
