// writeDomain_test.go pins the corrected settlement write domain: intents and
// data-layer rows are written, computed-layer targets refuse the whole flow.
package settle

import (
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestSettleWritesTradeIntent proves a positive cost becomes trade request rows
// (意向/标的/报价) and never the computed 成交额/结果 rows.
func TestSettleWritesTradeIntent(t *testing.T) {
	flow := baseFlow()
	deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
	completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	if completion.FlowID != flow.ID {
		t.Fatalf("completion = %+v", completion)
	}
	intent, ok := findChange(report.Changes, OP_SPEND, PATH_TRADE_INTENT)
	if !ok || intent.Value != TRADE_SIDE_BUY {
		t.Fatalf("%s = %+v, want %q", PATH_TRADE_INTENT, intent, TRADE_SIDE_BUY)
	}
	object, ok := findChange(report.Changes, OP_SPEND, PATH_TRADE_OBJECT)
	if !ok || object.Value != "食材" {
		t.Fatalf("%s = %+v, want 食材", PATH_TRADE_OBJECT, object)
	}
	price, ok := findChange(report.Changes, OP_SPEND, PATH_TRADE_PRICE)
	if !ok || price.Value != 10 {
		t.Fatalf("%s = %+v, want 10", PATH_TRADE_PRICE, price)
	}
	for _, change := range report.Changes {
		if change.Path == "交易.成交额" || change.Path == "交易.结果" {
			t.Fatalf("settlement wrote computed trade row %+v", change)
		}
	}
}

// TestSettleWritesInteractionIntent proves a consume flow writes the interaction
// request rows and leaves the material entity untouched.
func TestSettleWritesInteractionIntent(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Intents = []spec.IntentSpec{{Action: "吃", Target: "食材"}}
	deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	action, ok := findChange(report.Changes, OP_INTENT, PATH_INTERACT_ACTION)
	if !ok || action.Value != "吃" {
		t.Fatalf("%s = %+v, want 吃", PATH_INTERACT_ACTION, action)
	}
	target, ok := findChange(report.Changes, OP_INTENT, PATH_INTERACT_TARGET)
	if !ok || target.Value != "食材" {
		t.Fatalf("%s = %+v, want 食材", PATH_INTERACT_TARGET, target)
	}
	if _, ok := findChange(report.Changes, OP_INTENT, PATH_ITEM_QUANTITY); ok {
		t.Fatal("settlement wrote the consumed material quantity")
	}
}

// TestSettleDefaultInteractionAction proves a consume flow without an intent
// verb falls back to 使用 instead of inventing a verb.
func TestSettleDefaultInteractionAction(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Intents = nil
	deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	action, ok := findChange(report.Changes, OP_INTENT, PATH_INTERACT_ACTION)
	if !ok || action.Value != DEFAULT_INTERACT_ACTION {
		t.Fatalf("%s = %+v, want %q", PATH_INTERACT_ACTION, action, DEFAULT_INTERACT_ACTION)
	}
}

// TestSettleRejectsComputedTarget proves a computed-layer target refuses the
// whole flow and reports the offending field without any write.
func TestSettleRejectsComputedTarget(t *testing.T) {
	computed := []string{"代谢.胃内容", "行为.唤起", "睡眠.疲劳", "事件.对话", "状态.触发", "交易.成交额", "交易.结果"}
	for _, target := range computed {
		t.Run(target, func(t *testing.T) {
			flow := baseFlow()
			flow.Effects = []spec.EffectSpec{mappedEffect(target, spec.EFFECT_KIND_SET, 1)}
			deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))
			completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
			if !report.Failed() {
				t.Fatalf("Settle accepted computed target %q", target)
			}
			if completion != (Completion{}) {
				t.Fatalf("completion = %+v, want zero", completion)
			}
			if got := strings.Join(errorStrings(report), "; "); !strings.Contains(got, target) || !strings.Contains(got, "computed-layer") {
				t.Fatalf("error %q does not name %q as computed-layer", got, target)
			}
			assertRefused(t, flow, completion, report, store, ledger)
		})
	}
}

// TestSettleWritesDataLayer proves 标记./关系.数值./成长. targets are accepted.
func TestSettleWritesDataLayer(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Consume = nil
	flow.Produce = nil
	flow.Effects = []spec.EffectSpec{
		mappedEffect("标记.口渴", spec.EFFECT_KIND_BOOL, true),
		mappedEffect("关系.数值.信赖", spec.EFFECT_KIND_ADD, 3),
		mappedEffect("成长.经验", spec.EFFECT_KIND_SET, 12),
	}
	deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	for _, path := range []string{"标记.口渴", "关系.数值.信赖", "成长.经验"} {
		if _, ok := findChange(report.Changes, OP_EFFECT, path); !ok {
			t.Fatalf("missing data-layer change %q: %+v", path, report.Changes)
		}
	}
}

// errorStrings renders report errors for assertions.
func errorStrings(report Report) []string {
	out := make([]string, 0, len(report.Errors))
	for _, err := range report.Errors {
		if err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}
