// fromTick_test.go covers folding one tick's writes into net changes and the
// startup baseline seeding of long-lived state.
package observation

import (
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
)

func TestChangesFromWritesKeepsEarliestFromAndLatestTo(t *testing.T) {
	writes := []engine.EntityWrite{
		{EntityID: "猫", Path: "状态.身体", From: "健康", To: "疲惫", RuleID: "r1"},
		{EntityID: "猫", Path: "状态.身体", From: "疲惫", To: "虚弱", RuleID: "r2"},
		{EntityID: "猫", Path: "位置.场景", From: "书房", To: "卧室"},
		{EntityID: "狗", Path: "状态.身体", From: "健康", To: "疲惫"},
	}
	groups := ChangesFromWrites(writes)
	if len(groups) != 2 {
		t.Fatalf("应分成两个实体：%+v", groups)
	}
	cat := groups[0]
	if cat.Entity != "猫" || len(cat.Changes) != 2 {
		t.Fatalf("猫应有两条净变化：%+v", cat)
	}
	if cat.Changes[0].From != "健康" || cat.Changes[0].To != "虚弱" {
		t.Fatalf("同路径应保留最早 from 与最新 to：%+v", cat.Changes[0])
	}
	if len(cat.Changes[0].RuleIDs) != 2 || cat.Changes[0].RuleIDs[0] != "r1" || cat.Changes[0].RuleIDs[1] != "r2" {
		t.Fatalf("规则 id 应去重并保持顺序：%+v", cat.Changes[0].RuleIDs)
	}
}

func TestChangesFromWritesDropsNetZeroAndEmpty(t *testing.T) {
	writes := []engine.EntityWrite{
		{EntityID: "猫", Path: "状态.身体", From: 1, To: 2},
		{EntityID: "猫", Path: "状态.身体", From: 2, To: 1},
		{EntityID: "", Path: "状态.身体", From: 1, To: 2},
		{EntityID: "猫", Path: "", From: 1, To: 2},
	}
	if groups := ChangesFromWrites(writes); len(groups) != 0 {
		t.Fatalf("净变化为零或无路径的写入应被丢弃：%+v", groups)
	}
}

func TestEventsForEntityBuildsScopedEvents(t *testing.T) {
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	events := EventsForEntity("猫", []Change{
		{Path: "状态.身体", From: "疲惫", To: "虚弱", RuleIDs: []string{"r1"}},
		{Path: "位置.场景", From: "书房", To: "书房"},
		{Path: "", From: "x", To: "y"},
	}, "tick-1", at)
	if len(events) != 1 {
		t.Fatalf("只应保留一条有效事件：%+v", events)
	}
	event := events[0]
	if event.Scope != "entity:猫" || event.Path != "状态.身体" || event.TickID != "tick-1" {
		t.Fatalf("事件字段不符：%+v", event)
	}
	if event.At != "2026-11-14T08:00:00Z" {
		t.Fatalf("时间戳应为 UTC RFC3339，得到 %s", event.At)
	}
}

func TestSeedEntityWritesNonNoiseLeafBaseline(t *testing.T) {
	bus := NewBus()
	policy := testPolicy()
	state := map[string]any{
		"身份": map[string]any{"名字": "猫猫", "种族": "猫娘"},
		"状态": map[string]any{"身体": "健康"},
		"代谢": map[string]any{"体温": 370},
	}
	if count := SeedEntity(bus, policy, "猫", state); count != 3 {
		t.Fatalf("应写入 3 条基线（噪声路径除外），得到 %d", count)
	}
	entries, _, _ := State(bus, "entity:猫", policy)
	if len(entries) != 3 {
		t.Fatalf("基线应进入状态投影：%+v", entries)
	}
	paths := map[string]string{}
	for _, entry := range entries {
		paths[entry.Path] = ""
	}
	for _, want := range []string{"身份.名字", "身份.种族", "状态.身体"} {
		if _, exists := paths[want]; !exists {
			t.Fatalf("缺少基线路径 %s：%+v", want, paths)
		}
	}
	selection := Select(bus, "entity:猫", 0, 0, policy)
	if len(selection.Events) != 0 {
		t.Fatalf("基线不得进入变化推送：%+v", selection.Events)
	}
}

func TestSeedEntityIgnoresEmptyState(t *testing.T) {
	if count := SeedEntity(NewBus(), testPolicy(), "猫", nil); count != 0 {
		t.Fatalf("空状态不应写入基线，得到 %d", count)
	}
}
