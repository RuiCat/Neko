// gate_test.go pins the deterministic gate: replay integrity plus the frozen
// reference set structure.
package regression

import (
	"testing"

	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/referenceset"
)

func validRecord() journal.TurnRecord {
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

func TestGateCleanPasses(t *testing.T) {
	result := Gate([]journal.TurnRecord{validRecord()}, nil)
	if !result.Passed {
		t.Fatalf("干净记录应通过: %+v", result.Checks)
	}
}

func TestGateTamperedFails(t *testing.T) {
	record := validRecord()
	record.Compensation.State = map[string]any{"猫猫.状态.情绪": "愤怒"}
	result := Gate([]journal.TurnRecord{record}, nil)
	if result.Passed {
		t.Fatalf("篡改记录应失败: %+v", result.Checks)
	}
}

func TestGateEmptyReferenceSkips(t *testing.T) {
	result := Gate([]journal.TurnRecord{validRecord()}, nil)
	for _, check := range result.Checks {
		if check.Name == "reference_set" && !check.Passed {
			t.Fatalf("空参照集应跳过而非失败: %+v", check)
		}
	}
}

func TestGateBadReferenceFails(t *testing.T) {
	references := []referenceset.Entry{
		{ID: "ref_1", ExpectedReactions: []string{"喵"}},
	}
	result := Gate([]journal.TurnRecord{validRecord()}, references)
	if result.Passed {
		t.Fatalf("参照集反应不足应失败: %+v", result.Checks)
	}
}

func TestGateValidReferencePasses(t *testing.T) {
	references := []referenceset.Entry{
		{ID: "ref_1", ExpectedReactions: []string{"喵", "蹭"}},
	}
	result := Gate([]journal.TurnRecord{validRecord()}, references)
	if !result.Passed {
		t.Fatalf("合法参照集应通过: %+v", result.Checks)
	}
}
