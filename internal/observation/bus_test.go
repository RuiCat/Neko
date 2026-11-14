// bus_test.go covers seq assignment, cursor reads, ring trimming, scope
// eviction and the delete path of the observation bus.
package observation

import (
	"fmt"
	"sync"
	"testing"
)

func TestBusAppendAssignsSeqAndVersion(t *testing.T) {
	bus := NewBus()
	first := bus.Append(Event{Scope: "entity:猫", Path: "状态.身体", From: "疲惫", To: "虚弱"})
	second := bus.Append(Event{Scope: "entity:猫", Path: "位置.场景", From: "书房", To: "卧室"})
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("seq 应为 1/2，得到 %d/%d", first.Seq, second.Seq)
	}
	if first.At == "" {
		t.Fatalf("append 必须补时间戳")
	}
	if got := bus.Version("entity:猫"); got != 2 {
		t.Fatalf("version 应为 2，得到 %d", got)
	}
	if got := bus.Version("entity:狗"); got != 0 {
		t.Fatalf("未知 scope 的 version 应为 0，得到 %d", got)
	}
}

func TestBusStreamFiltersSinceAndLimit(t *testing.T) {
	bus := NewBus()
	for index := 0; index < 5; index++ {
		bus.Append(Event{Scope: "entity:猫", Path: fmt.Sprintf("p%d", index), To: index})
	}
	events := bus.Stream("entity:猫", 2, 0)
	if len(events) != 3 || events[0].Seq != 3 {
		t.Fatalf("since=2 应返回 3 条且首条 seq=3：%+v", events)
	}
	limited := bus.Stream("entity:猫", 0, 2)
	if len(limited) != 2 || limited[1].Seq != 2 {
		t.Fatalf("limit=2 应返回前两条：%+v", limited)
	}
	if got := bus.Stream("entity:狗", 0, 0); len(got) != 0 {
		t.Fatalf("未知 scope 应返回空：%+v", got)
	}
}

func TestBusTrimsToCapacity(t *testing.T) {
	bus := NewBus()
	for index := 0; index < STREAM_CAPACITY+10; index++ {
		bus.Append(Event{Scope: "entity:猫", Path: "代谢.体温", To: index})
	}
	events := bus.Stream("entity:猫", 0, 0)
	if len(events) != STREAM_CAPACITY {
		t.Fatalf("环形缓冲应保留 %d 条，得到 %d", STREAM_CAPACITY, len(events))
	}
	if events[len(events)-1].Seq != int64(STREAM_CAPACITY+10) {
		t.Fatalf("最新 seq 应为 %d，得到 %d", STREAM_CAPACITY+10, events[len(events)-1].Seq)
	}
}

func TestBusAppendAllKeepsOrderAndOneLock(t *testing.T) {
	bus := NewBus()
	stored := bus.AppendAll([]Event{
		{Scope: "entity:猫", Path: "a", To: 1},
		{Scope: "entity:猫", Path: "b", To: 2},
	})
	if len(stored) != 2 || stored[0].Seq != 1 || stored[1].Seq != 2 {
		t.Fatalf("批量写入应保持顺序：%+v", stored)
	}
	if got := bus.AppendAll(nil); got != nil {
		t.Fatalf("空批次应返回 nil：%+v", got)
	}
}

func TestBusEvictsLeastRecentlyWrittenScope(t *testing.T) {
	bus := NewBus()
	for index := 0; index < MAX_SCOPES+2; index++ {
		bus.Append(Event{Scope: fmt.Sprintf("entity:e%d", index), Path: "p", To: index})
	}
	if got := bus.Version("entity:e0"); got != 0 {
		t.Fatalf("最早的 scope 应被淘汰，得到 version=%d", got)
	}
	if got := bus.Version(fmt.Sprintf("entity:e%d", MAX_SCOPES+1)); got == 0 {
		t.Fatalf("最新 scope 应保留")
	}
}

func TestBusScopesAndDelete(t *testing.T) {
	bus := NewBus()
	bus.Append(Event{Scope: "entity:猫", Path: "p", To: 1})
	bus.Append(Event{Scope: "global", Path: "p", To: 1})
	if got := bus.Scopes(); len(got) != 2 {
		t.Fatalf("应有两个 scope：%+v", got)
	}
	if got := bus.ScopesForEntity("狗"); len(got) != 1 || got[0] != "entity:狗" {
		t.Fatalf("无事件的实体也应返回其 scope：%+v", got)
	}
	bus.Delete("entity:猫")
	bus.Delete("entity:猫")
	if got := bus.Version("entity:猫"); got != 0 {
		t.Fatalf("删除后 version 应为 0，得到 %d", got)
	}
	if got := len(bus.Scopes()); got != 1 {
		t.Fatalf("删除后应剩一个 scope，得到 %d", got)
	}
}

func TestBusConcurrentAppendIsRaceFree(t *testing.T) {
	bus := NewBus()
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for index := 0; index < 50; index++ {
				bus.Append(Event{Scope: "entity:猫", Path: fmt.Sprintf("w%d", worker), To: index})
			}
		}(worker)
	}
	wait.Wait()
	if got := bus.Version("entity:猫"); got != 400 {
		t.Fatalf("并发写入后 version 应为 400，得到 %d", got)
	}
}
