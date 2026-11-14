// select_test.go covers delta selection (noise, threshold, coalesce, ordering,
// budget, cursor) and the folded state projection.
package observation

import (
	"fmt"
	"testing"
)

// testPolicy is a small policy with one critical, one normal and one noise path.
func testPolicy() *Policy {
	return &Policy{
		Version:     2,
		DefaultTier: TierNormal,
		Rules: []Rule{
			{Pattern: "状态.*", Tier: TierCritical},
			{Pattern: "代谢.*", Tier: TierNoise},
			{Pattern: "循环.心率", Tier: TierNoise, Threshold: 2},
			{Pattern: "位置.*", Tier: TierNormal},
		},
		Budget: Budget{MaxItems: 4, MaxChars: 100, CoalesceMs: 60000},
	}
}

func TestSelectDropsNoiseAndCountsIt(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "代谢.体温", To: 370})
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "疲惫", To: "虚弱"})
	selection := Select(bus, "entity:猫", 0, 0, testPolicy())
	if len(selection.Events) != 1 || selection.Events[0].Path != "状态.身体" {
		t.Fatalf("噪声应被丢弃：%+v", selection.Events)
	}
	if selection.Events[0].Tier != TierCritical {
		t.Fatalf("tier 应为 critical，得到 %s", selection.Events[0].Tier)
	}
	if selection.Dropped != 1 {
		t.Fatalf("dropped 应为 1，得到 %d", selection.Dropped)
	}
	if selection.Cursor != 2 {
		t.Fatalf("cursor 应为最后一条事件 seq=2，得到 %d", selection.Cursor)
	}
}

func TestSelectAdvancesCursorPastNoiseOnly(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "代谢.体温", To: 370})
	bus.Append(Event{Scope: "entity:猫", Path: "循环.心率", To: 90})
	selection := Select(bus, "entity:猫", 0, 0, testPolicy())
	if len(selection.Events) != 0 {
		t.Fatalf("全是噪声时不应有事件：%+v", selection.Events)
	}
	if selection.Cursor != 2 {
		t.Fatalf("无事件时 cursor 应推进到 version=2，得到 %d", selection.Cursor)
	}
}

func TestSelectSkipsSubThresholdNumericChange(t *testing.T) {
	policy := testPolicy()
	policy.Rules = append(policy.Rules, Rule{Pattern: "体温", Tier: TierNormal, Threshold: 3})
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "体温", From: 370, To: 371})
	bus.Append(Event{Scope: "entity:猫", Path: "体温", From: 371, To: 376})
	selection := Select(bus, "entity:猫", 0, 0, policy)
	if len(selection.Events) != 1 {
		t.Fatalf("小于阈值的变化应被跳过，只留一条：%+v", selection.Events)
	}
	if selection.Events[0].From != 371 || selection.Events[0].To != 376 {
		t.Fatalf("留下的应是越过阈值的那条：%+v", selection.Events[0])
	}
}

func TestSelectCoalescesNormalEventsInsideWindow(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: "书房", To: "走廊", At: "2026-11-14T08:00:00Z"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: "走廊", To: "卧室", At: "2026-11-14T08:00:30Z"})
	selection := Select(bus, "entity:猫", 0, 0, testPolicy())
	if len(selection.Events) != 1 {
		t.Fatalf("窗口内应合并为一条：%+v", selection.Events)
	}
	merged := selection.Events[0]
	if merged.From != "书房" || merged.To != "卧室" || merged.Seq != 2 {
		t.Fatalf("合并应保留最初 from 与最新 to/seq：%+v", merged)
	}
}

func TestSelectDoesNotCoalesceCriticalOrOutOfWindow(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "a", To: "b", At: "2026-11-14T08:00:00Z"})
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "b", To: "c", At: "2026-11-14T08:00:10Z"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: "a", To: "b", At: "2026-11-14T08:00:00Z"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: "b", To: "c", At: "2026-11-14T09:00:00Z"})
	selection := Select(bus, "entity:猫", 0, 0, testPolicy())
	if len(selection.Events) != 4 {
		t.Fatalf("critical 不合并、窗口外不合并，应剩 4 条：%+v", selection.Events)
	}
	if selection.Events[0].Path != "状态.身体" || selection.Events[1].Path != "状态.身体" {
		t.Fatalf("critical 应排在前面：%+v", selection.Events)
	}
}

func TestSelectOrdersCriticalFirstThenCapsBudget(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", To: "书房"})
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", To: "虚弱"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.房间", To: "卧室"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.楼层", To: "二楼"})
	bus.Append(Event{Scope: "entity:猫", Path: "状态.情绪", To: "低落"})
	policy := testPolicy()
	policy.Budget.MaxItems = 3
	selection := Select(bus, "entity:猫", 0, 0, policy)
	if !selection.Truncated {
		t.Fatalf("超出预算应标记 truncated")
	}
	if len(selection.Events) != 3 {
		t.Fatalf("应截断到 3 条，得到 %d", len(selection.Events))
	}
	if selection.Events[0].Tier != TierCritical || selection.Events[1].Tier != TierCritical {
		t.Fatalf("critical 必须先出：%+v", selection.Events)
	}
	if selection.Cursor != selection.Events[2].Seq {
		t.Fatalf("cursor 应指向最后一条返回事件：%d vs %d", selection.Cursor, selection.Events[2].Seq)
	}
}

func TestSelectClampsSinceAboveVersion(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", To: "虚弱"})
	selection := Select(bus, "entity:猫", 99, 0, testPolicy())
	if len(selection.Events) != 0 || selection.Cursor != 1 {
		t.Fatalf("since 超过 version 应返回空并推进 cursor=1：%+v", selection)
	}
}

func TestSelectLimitNarrowsBudget(t *testing.T) {
	bus := NewBus()
	for index := 0; index < 6; index++ {
		bus.Append(Event{Scope: "entity:猫", Path: "状态.p", To: index})
	}
	selection := Select(bus, "entity:猫", 0, 2, testPolicy())
	if len(selection.Events) != 2 || !selection.Truncated {
		t.Fatalf("limit=2 应截断到 2 条：%+v", selection)
	}
}

func TestStateFoldsLatestValuePerPath(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "健康", To: "疲惫"})
	bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "疲惫", To: "虚弱"})
	bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: nil, To: "卧室"})
	bus.Append(Event{Scope: "entity:猫", Path: "代谢.体温", To: 370})
	entries, version, truncated := State(bus, "entity:猫", testPolicy())
	if version != 4 || truncated {
		t.Fatalf("version 应为 4 且未截断：%d/%v", version, truncated)
	}
	if len(entries) != 2 {
		t.Fatalf("应折叠为 2 条（噪声被过滤）：%+v", entries)
	}
	if entries[0].Path != "状态.身体" || entries[0].Value != "虚弱" || entries[0].From != "疲惫" {
		t.Fatalf("critical 应在前且保留最新值/上一值：%+v", entries[0])
	}
	if entries[1].Path != "位置.场景" || entries[1].Value != "卧室" {
		t.Fatalf("normal 条目值不符：%+v", entries[1])
	}
}

func TestStateCapsEntriesAndReportsTruncation(t *testing.T) {
	bus := NewBus()
	for index := 0; index < 6; index++ {
		bus.Append(Event{Scope: "entity:猫", Path: fmt.Sprintf("位置.p%d", index), To: index})
	}
	policy := testPolicy()
	policy.Budget.MaxItems = 3
	entries, _, truncated := State(bus, "entity:猫", policy)
	if len(entries) != 3 || !truncated {
		t.Fatalf("应按预算截断到 3 条：%d/%v", len(entries), truncated)
	}
}
