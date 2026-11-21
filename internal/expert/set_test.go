// set_test.go pins the per-entity isolation and the own-session recall filter.
package expert

import (
	"context"
	"testing"

	"github.com/RuiCat/Neko/internal/gate"
)

// TestSetKeepsEntityPoolsApart proves entities never share memory while every
// session of one entity reads the same pool.
func TestSetKeepsEntityPoolsApart(t *testing.T) {
	experts := NewDefaultSet(4)
	if !experts.Remember("猫猫", gate.EXPERT_EVENT, Entry{
		SnapshotID: "evt_cat", SessionID: "s-main", Content: "猫猫的记忆", Weight: 0.9,
	}) {
		t.Fatal("猫猫 的写入被拒绝")
	}
	if entities := experts.Entities(); len(entities) != 1 || entities[0] != "猫猫" {
		t.Fatalf("实体列表 %v", entities)
	}
	children, err := experts.Compensate(context.Background(), "猫猫", []string{gate.EXPERT_EVENT}, Input{SessionID: "s-child"})
	if err != nil || len(children) != 1 || children[0].SourceSnapshotID != "evt_cat" {
		t.Fatalf("同一实体的子会话应读到实体池：%v,%v", children, err)
	}
	own, err := experts.Compensate(context.Background(), "猫猫", []string{gate.EXPERT_EVENT}, Input{SessionID: "s-main"})
	if err != nil || len(own) != 0 {
		t.Fatalf("本会话自己产生的条目不应召回：%v,%v", own, err)
	}
	other, err := experts.Compensate(context.Background(), "梦梦", []string{gate.EXPERT_EVENT}, Input{SessionID: "s-child"})
	if err != nil || len(other) != 0 {
		t.Fatalf("梦梦 不应读到 猫猫 的记忆：%v,%v", other, err)
	}
	legacy, err := experts.Compensate(context.Background(), "猫猫", []string{gate.EXPERT_EVENT}, Input{})
	if err != nil || len(legacy) != 1 {
		t.Fatalf("空会话应按未知会话处理（历史条目可召回）：%v,%v", legacy, err)
	}
}

// TestSetDecayAllIsPerEntity pins the nested decay result.
func TestSetDecayAllIsPerEntity(t *testing.T) {
	experts := NewDefaultSet(4)
	experts.Remember("猫猫", gate.EXPERT_EVENT, Entry{SnapshotID: "evt_old", Content: "旧", Weight: 0.06})
	experts.Remember("梦梦", gate.EXPERT_EXPRESSION, Entry{SnapshotID: "evt_keep", Content: "新", Weight: 0.9})
	removed := experts.DecayAll(0.5)
	if removed["猫猫"][gate.EXPERT_EVENT] != 1 {
		t.Fatalf("猫猫 的事件记忆应移除 1 条：%v", removed)
	}
	if removed["梦梦"][gate.EXPERT_EXPRESSION] != 0 {
		t.Fatalf("梦梦 的表达偏好不应被移除：%v", removed)
	}
}

// TestRecallExpertSkipsCurrentSessionEntries pins the entry-level filter.
func TestRecallExpertSkipsCurrentSessionEntries(t *testing.T) {
	memory := NewRecallExpert(gate.EXPERT_EVENT, 4, 3)
	memory.Remember(Entry{SnapshotID: "evt_own", SessionID: "s-1", Content: "本会话", Weight: 0.9})
	memory.Remember(Entry{SnapshotID: "evt_other", SessionID: "s-2", Content: "别的会话", Weight: 0.8})
	memory.Remember(Entry{SnapshotID: "evt_legacy", Content: "旧条目", Weight: 0.7})
	fragments, err := memory.Compensate(context.Background(), Input{SessionID: "s-1"})
	if err != nil || len(fragments) != 2 {
		t.Fatalf("应跳过本会话条目：%v,%v", fragments, err)
	}
	if fragments[0].SourceSnapshotID != "evt_other" || fragments[1].SourceSnapshotID != "evt_legacy" {
		t.Fatalf("保留顺序错误：%+v", fragments)
	}
	all, err := memory.Compensate(context.Background(), Input{})
	if err != nil || len(all) != 3 {
		t.Fatalf("无会话时全部可召回：%v,%v", all, err)
	}
}
