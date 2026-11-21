// assemble_test.go pins the fail-closed assembly path and the package rules.
package compensation

import (
	"context"
	"errors"
	"testing"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

type staticGate struct {
	active []string
	err    error
}

func (g staticGate) Activate(gate.Features) ([]string, error) {
	return g.active, g.err
}

func testSnapshot() hardstate.Snapshot {
	return hardstate.Snapshot{
		SnapshotID: "evt_20261103_0001",
		TurnID:     "evt_20261103_0001",
		State:      map[string]any{"猫猫.状态.情绪": "平静"},
	}
}

func testBoard(t *testing.T) *blackboard.Board {
	t.Helper()
	board := blackboard.New()
	if err := board.SetSnapshot(testSnapshot()); err != nil {
		t.Fatal(err)
	}
	return board
}

func testRegistry(t *testing.T) *expert.Set {
	t.Helper()
	experts := expert.NewDefaultSet(4)
	experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{
		SnapshotID: "evt_20261102_0009",
		Content:    "昨晚的约定",
		Weight:     0.8,
	})
	return experts
}

func TestAssembleProducesValidPackage(t *testing.T) {
	assembler := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, testRegistry(t))
	pkg, err := assembler.Assemble(context.Background(), Input{
		Entity:    "猫猫",
		UserInput: "记得吗",
		Board:     testBoard(t),
		Situation: "室内·夜间·两人",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.SnapshotID != "evt_20261103_0001" || pkg.StateSource != STATE_SOURCE {
		t.Fatalf("包来源错误：%+v", pkg)
	}
	if pkg.State["猫猫.状态.情绪"] != "平静" {
		t.Fatalf("状态未从快照投影：%+v", pkg.State)
	}
	if len(pkg.Memory) != 1 || pkg.Memory[0].SourceSnapshotID != "evt_20261102_0009" {
		t.Fatalf("记忆片段错误：%+v", pkg.Memory)
	}
	if err := Validate(pkg, testSnapshot()); err != nil {
		t.Fatalf("自产包未通过校验: %v", err)
	}
}

func TestAssembleFailsWithoutSnapshot(t *testing.T) {
	assembler := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, testRegistry(t))
	if _, err := assembler.Assemble(context.Background(), Input{Entity: "猫猫", Board: blackboard.New()}); err == nil {
		t.Fatal("没有快照时应失败")
	}
}

func TestAssemblePropagatesGateError(t *testing.T) {
	assembler := NewAssembler(staticGate{err: errors.New("boom")}, testRegistry(t))
	if _, err := assembler.Assemble(context.Background(), Input{Entity: "猫猫", Board: testBoard(t)}); err == nil {
		t.Fatal("门控错误应向上返回")
	}
}

func TestAssembleCapsFragments(t *testing.T) {
	experts := expert.NewDefaultSet(4)
	experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{SnapshotID: "a", Content: "一", Weight: 1})
	experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{SnapshotID: "b", Content: "二", Weight: 1})
	assembler := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, experts)
	assembler.MaxFragments = 1
	pkg, err := assembler.Assemble(context.Background(), Input{Entity: "猫猫", Board: testBoard(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Memory) != 1 {
		t.Fatalf("片段上限失效：%d", len(pkg.Memory))
	}
}

func TestValidateRejectsMutations(t *testing.T) {
	pkg, err := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, testRegistry(t)).
		Assemble(context.Background(), Input{Entity: "猫猫", Board: testBoard(t)})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*journal.Compensation)
	}{
		{"状态被改", func(p *journal.Compensation) { p.State["猫猫.状态.情绪"] = "烦躁" }},
		{"来源错误", func(p *journal.Compensation) { p.StateSource = "local_model" }},
		{"快照不匹配", func(p *journal.Compensation) { p.SnapshotID = "evt_other" }},
		{"片段缺来源", func(p *journal.Compensation) { p.Memory[0].SourceSnapshotID = "" }},
		{"类型非法", func(p *journal.Compensation) { p.Memory[0].Type = "invented" }},
		{"重建权重过高", func(p *journal.Compensation) {
			p.Memory = append(p.Memory, journal.MemoryFragment{
				Content: "重建", SourceSnapshotID: "evt_20261102_0009", Type: "reconstructed", Weight: 0.9,
			})
		}},
	}
	for _, testCase := range cases {
		mutated := clonePackage(pkg)
		testCase.mutate(&mutated)
		if err := Validate(mutated, testSnapshot()); err == nil {
			t.Fatalf("%s 应被拒绝", testCase.name)
		}
	}
}

func clonePackage(pkg journal.Compensation) journal.Compensation {
	cloned := pkg
	cloned.State = map[string]any{}
	for key, value := range pkg.State {
		cloned.State[key] = value
	}
	cloned.Memory = append([]journal.MemoryFragment(nil), pkg.Memory...)
	return cloned
}

func TestDedupeFragmentsKeepsHighestWeight(t *testing.T) {
	fragments := dedupeFragments([]expert.Fragment{
		{Content: "同一件事", SourceSnapshotID: "evt_1", Type: "retrieved", Weight: 0.5},
		{Content: "同一件事", SourceSnapshotID: "evt_1", Type: "retrieved", Weight: 0.9},
		{Content: "另一件事", SourceSnapshotID: "evt_2", Type: "retrieved", Weight: 0.6},
	})
	if len(fragments) != 2 {
		t.Fatalf("去重后 %d 条，期望 2", len(fragments))
	}
	if fragments[0].Weight != 0.9 {
		t.Fatalf("应保留最高权重，得到 %v", fragments[0].Weight)
	}
}

// captureGate records the features the assembler hands to the gate.
type captureGate struct {
	features gate.Features
}

// Activate stores the incoming features and returns a valid activation.
func (g *captureGate) Activate(features gate.Features) ([]string, error) {
	g.features = features
	return []string{gate.EXPERT_EVENT}, nil
}

// TestAssembleKeepsEntityMemoriesSeparate proves one entity never recalls
// another entity's private working memory.
func TestAssembleKeepsEntityMemoriesSeparate(t *testing.T) {
	assembler := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, testRegistry(t))
	pkg, err := assembler.Assemble(context.Background(), Input{Entity: "梦梦", Board: testBoard(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Memory) != 0 {
		t.Fatalf("梦梦 不应召回 猫猫 的记忆：%+v", pkg.Memory)
	}
}

// TestAssembleSkipsOwnSessionEntries proves the entity pool still filters the
// reading session's own turn, while sibling sessions of the same entity can
// recall it.
func TestAssembleSkipsOwnSessionEntries(t *testing.T) {
	experts := expert.NewDefaultSet(4)
	experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{
		SnapshotID: "evt_self", SessionID: "s-main", Content: "用户: 在吗 | 助手: 在的", Weight: 0.8,
	})
	assembler := NewAssembler(staticGate{active: []string{gate.EXPERT_EVENT}}, experts)
	own, err := assembler.Assemble(context.Background(), Input{Entity: "猫猫", SessionID: "s-main", Board: testBoard(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(own.Memory) != 0 {
		t.Fatalf("本会话自己产生的条目不应召回：%+v", own.Memory)
	}
	sibling, err := assembler.Assemble(context.Background(), Input{Entity: "猫猫", SessionID: "s-child", Board: testBoard(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(sibling.Memory) != 1 || sibling.Memory[0].SourceSnapshotID != "evt_self" {
		t.Fatalf("同一实体的别的会话应能召回：%+v", sibling.Memory)
	}
}

// TestAssembleDerivesFeaturesBeforeGate proves the live signals reach the gate
// instead of the raw "turn" hint the callers pass.
func TestAssembleDerivesFeaturesBeforeGate(t *testing.T) {
	capture := &captureGate{}
	assembler := NewAssembler(capture, testRegistry(t))
	_, err := assembler.Assemble(context.Background(), Input{
		Entity:    "猫猫",
		UserInput: "你为什么这么晚才回来？",
		Board:     testBoard(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if capture.features.EventKind != "question" {
		t.Fatalf("事件类型未从用户文本推导: %q", capture.features.EventKind)
	}
	if capture.features.Summary == "" {
		t.Fatal("摘要未从用户文本填充")
	}
	if _, ok := capture.features.Affect["valence"]; !ok {
		t.Fatalf("affect 未从快照推导: %+v", capture.features.Affect)
	}
}
