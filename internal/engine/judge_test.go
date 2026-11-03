// judge_test.go pins the read-only coverage verdict: legal, illegal and
// uncovered, plus the no-mutation guarantee.
package engine

import (
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
)

func judgeFixture(t *testing.T) (*StateMachine, *entity.Entity) {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	ent, err := store.Create("猫猫", "NPC", map[string]any{"状态": map[string]any{"情绪": "平静"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_mood", Match: "猫猫", Enabled: true,
		Branches: []Branch{{Then: []Op{{Op: "set", Path: "状态.情绪", Value: "烦躁"}}}},
	})
	return machine, ent
}

func TestJudgeLegalWhenRuleCoversPath(t *testing.T) {
	machine, _ := judgeFixture(t)
	verdict, err := machine.Judge(Candidate{EntityID: "猫猫", Path: "状态.情绪", Proposed: "烦躁"})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Status != VERDICT_LEGAL || verdict.RuleID != "r_mood" {
		t.Fatalf("裁决错误：%+v", verdict)
	}
}

func TestJudgeAcceptsQualifiedAndPlaceholderPaths(t *testing.T) {
	machine, _ := judgeFixture(t)
	for _, path := range []string{"猫猫.状态.情绪", "{e}.状态.情绪"} {
		verdict, _ := machine.Judge(Candidate{EntityID: "猫猫", Path: path})
		if verdict.Status != VERDICT_LEGAL {
			t.Fatalf("路径 %q 应命中规则：%+v", path, verdict)
		}
	}
}

func TestJudgeUncoveredAndIllegal(t *testing.T) {
	machine, _ := judgeFixture(t)
	uncovered, _ := machine.Judge(Candidate{EntityID: "猫猫", Path: "状态.信任值"})
	if uncovered.Status != VERDICT_UNCOVERED {
		t.Fatalf("未覆盖路径应判 uncovered：%+v", uncovered)
	}
	missing, _ := machine.Judge(Candidate{EntityID: "梦梦", Path: "状态.情绪"})
	if missing.Status != VERDICT_ILLEGAL {
		t.Fatalf("不存在的实体应判 illegal：%+v", missing)
	}
	emptyPath, _ := machine.Judge(Candidate{EntityID: "猫猫", Path: " "})
	if emptyPath.Status != VERDICT_ILLEGAL {
		t.Fatalf("空路径应判 illegal：%+v", emptyPath)
	}
}

func TestJudgeIgnoresDisabledRules(t *testing.T) {
	machine, _ := judgeFixture(t)
	machine.AddRule(Rule{
		ID: "r_disabled", Match: "猫猫", Enabled: false,
		Branches: []Branch{{Then: []Op{{Op: "set", Path: "状态.体力", Value: 1}}}},
	})
	verdict, _ := machine.Judge(Candidate{EntityID: "猫猫", Path: "状态.体力"})
	if verdict.Status != VERDICT_UNCOVERED {
		t.Fatalf("禁用规则不应提供覆盖：%+v", verdict)
	}
}

func TestJudgeDoesNotMutateState(t *testing.T) {
	machine, ent := judgeFixture(t)
	before := ent.Snapshot()
	rulesBefore := len(machine.Rules())
	if _, err := machine.Judge(Candidate{EntityID: "猫猫", Path: "状态.情绪", Proposed: "暴怒"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, ent.Snapshot()) {
		t.Fatalf("Judge 改动了状态：%v -> %v", before, ent.Snapshot())
	}
	if len(machine.Rules()) != rulesBefore {
		t.Fatalf("Judge 改动了规则数量：%d -> %d", rulesBefore, len(machine.Rules()))
	}
}
