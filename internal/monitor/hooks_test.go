// Observe-hook contracts: OnCreate never takes the store lock, tick observers
// are delivered outside tickMu without delaying the tick, and diagnostics
// notifications run after the store lock is released.
package monitor

import (
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// TestOnCreateCallbackNoStoreCall exercises C2: Store.Create holds its write
// lock while calling OnCreate, so a callback that calls any Store method would
// deadlock and this test would time out.
func TestOnCreateCallbackNoStoreCall(t *testing.T) {
	env := newTestEnv(t, Options{})
	done := make(chan error, 1)
	go func() {
		_, err := env.runtime.Store.Create("新猫", "NPC", map[string]any{"备注": "x"}, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("创建实体失败: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnCreate 回调阻塞：疑似在回调内调用了 Store 加锁方法")
	}
	created := env.runtime.Store.Get("新猫")
	if created == nil {
		t.Fatal("创建的实体不存在")
	}
	if err := created.Set("备注", "y"); err != nil {
		t.Fatalf("新实体的变更监听器不可写: %v", err)
	}
}

// TestTickObserverNonBlocking exercises C4: the observer is delivered outside
// tickMu, a slow observer never delays the tick and the tick still reaches the
// monitor's latest-tick record.
func TestTickObserverNonBlocking(t *testing.T) {
	env := newTestEnv(t, Options{})
	result, acquired, err := env.mcp.TryRunTick("", mcpserver.TickOptions{SkipSaveWhenUnchanged: true})
	if !acquired || err != nil {
		t.Fatalf("tick acquired=%v err=%v", acquired, err)
	}
	if result.Status == "" {
		t.Fatal("tick 结果为空")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := env.server.hub.LatestTickPayload(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tick 观察者未把结果投递给监视器")
		}
		time.Sleep(5 * time.Millisecond)
	}

	env.mcp.SetTickObserver(func(engine.TickResult) { time.Sleep(500 * time.Millisecond) })
	started := time.Now()
	if _, acquired, err := env.mcp.TryRunTick("", mcpserver.TickOptions{SkipSaveWhenUnchanged: true}); !acquired || err != nil {
		t.Fatalf("慢观察者 tick acquired=%v err=%v", acquired, err)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("慢观察者拖长 tick 墙钟: %s", elapsed)
	}
}

// TestNotifyNotUnderLock exercises C12: notify callbacks call back into the
// store, so they must run after Store.mu is released; add/update/remove
// operations are all distinguished.
func TestNotifyNotUnderLock(t *testing.T) {
	store := diagnostics.New(8)
	observed := make(chan string, 8)
	store.SetNotify(func(record diagnostics.Record, op string) {
		_ = store.Generation()
		observed <- op
	})
	record := diagnostics.Record{Code: "c", Script: "s", Message: "m", Severity: diagnostics.SEVERITY_WARNING}
	added := make(chan struct{})
	go func() {
		defer close(added)
		store.Add(record)
	}()
	select {
	case <-added:
	case <-time.After(2 * time.Second):
		t.Fatal("Add 阻塞：notify 在 Store.mu 内回调")
	}
	if op := nextOp(t, observed); op != "added" {
		t.Fatalf("首次 op = %s", op)
	}
	store.Add(record)
	if op := nextOp(t, observed); op != "updated" {
		t.Fatalf("重复 op = %s", op)
	}
	store.ClearByScript("s")
	if op := nextOp(t, observed); op != "removed" {
		t.Fatalf("清除 op = %s", op)
	}
	if len(store.Snapshot(diagnostics.Filter{Script: "s"}, 0)) != 0 {
		t.Fatal("ClearByScript 后仍残留记录")
	}
}

// nextOp waits for one observed operation label.
func nextOp(t *testing.T, observed <-chan string) string {
	t.Helper()
	select {
	case op := <-observed:
		return op
	case <-time.After(2 * time.Second):
		t.Fatal("没有收到诊断通知")
		return ""
	}
}
