// ruleGate_test.go pins the deterministic gate contract: 2–3 experts, stable
// order, event routing and the affect rules.
package gate

import "testing"

func TestEmptyFeaturesFallBackToTwoExperts(t *testing.T) {
	active, err := NewRuleGate().Activate(Features{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EXPERT_EVENT, EXPERT_EXPRESSION}
	if len(active) != len(want) {
		t.Fatalf("专家数 %d，期望 %d：%v", len(active), len(want), active)
	}
	for index, name := range want {
		if active[index] != name {
			t.Fatalf("顺序 %v，期望 %v", active, want)
		}
	}
}

func TestQuestionActivatesKnowledge(t *testing.T) {
	active, err := NewRuleGate().Activate(Features{EventKind: "question"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(active, EXPERT_KNOWLEDGE) {
		t.Fatalf("问题应激活知识边界：%v", active)
	}
	if len(active) < MIN_ACTIVE_EXPERTS || len(active) > MAX_ACTIVE_EXPERTS {
		t.Fatalf("专家数越界：%v", active)
	}
}

func TestLowTrustBringsRelationship(t *testing.T) {
	active, err := NewRuleGate().Activate(Features{EventKind: "turn", Affect: map[string]float64{"trust": 0.1}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(active, EXPERT_RELATION) {
		t.Fatalf("低信任应激活关系记忆：%v", active)
	}
}

func TestCapAtThreeExperts(t *testing.T) {
	features := Features{
		EventKind: "conflict",
		Summary:   "刚才的争执",
		Affect:    map[string]float64{"trust": 0.1, "valence": -0.9},
		HardState: map[string]any{"猫猫.状态.知识边界": "不知道"},
	}
	active, err := NewRuleGate().Activate(features)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != MAX_ACTIVE_EXPERTS {
		t.Fatalf("专家数 %d，期望 %d：%v", len(active), MAX_ACTIVE_EXPERTS, active)
	}
}

func TestDeterministicAcrossCalls(t *testing.T) {
	gate := NewRuleGate()
	features := Features{EventKind: "emotion", Summary: "深夜", Affect: map[string]float64{"valence": 0.8}}
	first, _ := gate.Activate(features)
	for attempt := 0; attempt < 5; attempt++ {
		again, _ := gate.Activate(features)
		if len(again) != len(first) {
			t.Fatalf("长度不稳定：%v vs %v", first, again)
		}
		for index := range first {
			if again[index] != first[index] {
				t.Fatalf("顺序不稳定：%v vs %v", first, again)
			}
		}
	}
}
