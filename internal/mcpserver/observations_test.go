// observations_test.go proves direct writes and ticks both reach the
// observation bus, and that redundant writes stay silent.
package mcpserver

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/observation"
)

// wireObservations attaches a fresh bus and the built-in policy to the fixture.
func wireObservations(t *testing.T, fx *tickFixture) *observation.Bus {
	t.Helper()
	bus := observation.NewBus()
	fx.server.SetObservations(bus, observation.DefaultPolicy())
	return bus
}

func TestDirectWriteReachesObservationBus(t *testing.T) {
	fx := newTickFixture(t)
	bus := wireObservations(t, fx)
	if _, err := fx.server.WriteEntityPaths("猫猫", map[string]any{"状态.情绪": "开心"}, "test"); err != nil {
		t.Fatalf("write: %v", err)
	}
	events := bus.Stream(observation.ScopeForEntity("猫猫"), 0, 0)
	if len(events) != 1 {
		t.Fatalf("直接写入应产生一条事件：%+v", events)
	}
	if events[0].Path != "状态.情绪" || events[0].To != "开心" || events[0].TickID != "direct:test" {
		t.Fatalf("事件字段不符：%+v", events[0])
	}
	// 同值重写不产生净变化，订阅推送不重复内容。
	if _, err := fx.server.WriteEntityPaths("猫猫", map[string]any{"状态.情绪": "开心"}, "test"); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := len(bus.Stream(observation.ScopeForEntity("猫猫"), 0, 0)); got != 1 {
		t.Fatalf("同值重写不应新增事件，得到 %d 条", got)
	}
}

func TestRunTickFeedsObservationBus(t *testing.T) {
	fx := newTickFixture(t)
	bus := wireObservations(t, fx)
	fx.machine.AddRule(engine.Rule{
		ID: "m1.observe", Match: "[*]", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: "身体.体力", Value: 40}}}},
	})
	if _, err := fx.server.RunTick("", TickOptions{}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	events := bus.Stream(observation.ScopeForEntity("猫猫"), 0, 0)
	if len(events) != 1 {
		t.Fatalf("一个 tick 内同路径多次写入应折叠成一条：%+v", events)
	}
	if events[0].Path != "身体.体力" || events[0].From != 50 || events[0].To != 40 {
		t.Fatalf("事件应为 50 → 40：%+v", events[0])
	}
	if events[0].TickID == "" || events[0].TickID == "direct:" {
		t.Fatalf("tick 事件应带 tick id：%q", events[0].TickID)
	}
	// 值未变的第二 tick 不产生事件。
	if _, err := fx.server.RunTick("", TickOptions{}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if got := len(bus.Stream(observation.ScopeForEntity("猫猫"), 0, 0)); got != 1 {
		t.Fatalf("无变化的 tick 不应新增事件，得到 %d 条", got)
	}
}

func TestObservationReaderFallsBackToEmpty(t *testing.T) {
	fx := newTickFixture(t)
	if bus := fx.server.Observations(); bus != nil {
		t.Fatalf("未接线时总线应为 nil：%+v", bus)
	}
	policy := fx.server.ObservationPolicy()
	if policy == nil || policy.BudgetOf().MaxItems != observation.DEFAULT_MAX_ITEMS {
		t.Fatalf("未接线时应回落内置策略：%+v", policy)
	}
}
