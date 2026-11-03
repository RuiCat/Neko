// expert_test.go pins context bounds, decay, recall fragments and registry
// ordering.
package expert

import (
	"context"
	"testing"

	"github.com/RuiCat/Neko/internal/gate"
)

func TestContextEvictsOldest(t *testing.T) {
	working := NewContext(2)
	working.Remember(Entry{Content: "a", Weight: 1})
	working.Remember(Entry{Content: "b", Weight: 1})
	working.Remember(Entry{Content: "c", Weight: 1})
	recent := working.Recent(0)
	if len(recent) != 2 || recent[0].Content != "b" || recent[1].Content != "c" {
		t.Fatalf("容量淘汰错误：%v", recent)
	}
	if working.Len() != 2 {
		t.Fatalf("长度 %d", working.Len())
	}
}

func TestDecayDropsLowWeights(t *testing.T) {
	working := NewContext(4)
	working.Remember(Entry{Content: "keep", Weight: 1})
	working.Remember(Entry{Content: "drop", Weight: 0.06})
	if removed := working.Decay(0.5); removed != 1 {
		t.Fatalf("移除 %d 条，期望 1", removed)
	}
	if working.Len() != 1 {
		t.Fatalf("剩余 %d 条", working.Len())
	}
}

func TestRecallExpertFragments(t *testing.T) {
	memory := NewRecallExpert(gate.EXPERT_EVENT, 4, 2)
	memory.Remember(Entry{SnapshotID: "evt_1", Content: "昨晚的约定", Weight: 0.8})
	fragments, err := memory.Compensate(context.Background(), Input{UserInput: "记得吗"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 1 {
		t.Fatalf("片段数 %d", len(fragments))
	}
	fragment := fragments[0]
	if fragment.Type != "retrieved" || fragment.SourceSnapshotID != "evt_1" || fragment.Weight != 0.8 {
		t.Fatalf("片段字段错误：%+v", fragment)
	}
}

func TestRecallExpertWithoutEntriesIsSilent(t *testing.T) {
	memory := NewRecallExpert(gate.EXPERT_SELF, 4, 2)
	fragments, err := memory.Compensate(context.Background(), Input{})
	if err != nil || len(fragments) != 0 {
		t.Fatalf("空上下文应返回 0 片段,nil；得到 %v,%v", fragments, err)
	}
}

func TestRegistryOrderDuplicateAndUnknown(t *testing.T) {
	registry := NewDefaultRegistry(4)
	if ids := registry.IDs(); len(ids) != len(gate.DEFAULT_EXPERT_ORDER) {
		t.Fatalf("默认专家数 %d", len(ids))
	}
	if err := registry.Add(NewRecallExpert(gate.EXPERT_EVENT, 4, 2)); err == nil {
		t.Fatal("重复注册应报错")
	}
	if _, err := registry.Compensate(context.Background(), []string{"不存在"}, Input{}); err == nil {
		t.Fatal("未知专家应报错")
	}
	cat := NewRecallExpert(gate.EXPERT_EVENT, 4, 1)
	cat.Remember(Entry{SnapshotID: "evt_1", Content: "事件", Weight: 1})
	registry2 := NewRegistry()
	_ = registry2.Add(cat)
	fragments, err := registry2.Compensate(context.Background(), []string{gate.EXPERT_EVENT}, Input{})
	if err != nil || len(fragments) != 1 {
		t.Fatalf("补偿失败：%v,%v", fragments, err)
	}
}
