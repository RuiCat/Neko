// board_test.go pins the two blackboard invariants: last write wins and the
// serialized size cap rejects history-sized boards.
package blackboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/hardstate"
)

func TestLastWriteWins(t *testing.T) {
	board := New()
	if err := board.SetEvent(Event{ID: "e1", Kind: "turn"}); err != nil {
		t.Fatal(err)
	}
	if err := board.SetEvent(Event{ID: "e2", Kind: "question"}); err != nil {
		t.Fatal(err)
	}
	if got := board.Event(); got.ID != "e2" || got.Kind != "question" {
		t.Fatalf("事件未覆盖：%+v", got)
	}
	if err := board.SetSnapshot(hardstate.Snapshot{SnapshotID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := board.SetSnapshot(hardstate.Snapshot{SnapshotID: "s2"}); err != nil {
		t.Fatal(err)
	}
	if got := board.Snapshot(); got.SnapshotID != "s2" {
		t.Fatalf("快照未覆盖：%+v", got)
	}
}

func TestReaderHasNoSetters(t *testing.T) {
	readerType := reflect.TypeOf((*Reader)(nil)).Elem()
	for _, method := range []string{"SetEvent", "SetSnapshot", "SetConstraints"} {
		if _, ok := readerType.MethodByName(method); ok {
			t.Fatalf("Reader 不应暴露写方法 %s", method)
		}
	}
	var reader Reader = New()
	if reader.Snapshot().SnapshotID != "" {
		t.Fatal("空黑板应返回零值快照")
	}
}

func TestSizeCapRejectsAndRollsBack(t *testing.T) {
	board := New()
	if err := board.SetEvent(Event{ID: "small"}); err != nil {
		t.Fatal(err)
	}
	huge := Event{ID: "huge", Summary: strings.Repeat("猫", MAX_SERIALIZED_BYTES)}
	if err := board.SetEvent(huge); err == nil {
		t.Fatal("超大黑板应被拒绝")
	}
	if got := board.Event(); got.ID != "small" {
		t.Fatalf("拒绝后应回滚到上一个有效值，得到 %+v", got)
	}
}
