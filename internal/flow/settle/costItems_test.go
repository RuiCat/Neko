// costItems_test.go covers the C5 surfaces: the spend writes trade-kernel
// request rows (never the currency field), and consume/produce resolve material
// items by (name, scene, quantity) in both accepted item-view shapes.
package settle

import (
	"fmt"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestSettleCostThroughTrade asserts a positive cost becomes 交易.成交额 and
// 交易.结果 rows and that the currency field is never a change target (C5).
func TestSettleCostThroughTrade(t *testing.T) {
	flow := baseFlow()
	flow.Consume = nil
	flow.Produce = nil
	flow.Effects = nil
	flow.Cost = 10
	deps, _, _ := testDeps(NewSnapshot(nil))

	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	if len(report.Changes) != 2 {
		t.Fatalf("changes = %d, want 2 spend rows", len(report.Changes))
	}
	amount, ok := findChange(report.Changes, OP_SPEND, PATH_TRADE_AMOUNT)
	if !ok {
		t.Fatalf("missing %q spend row", PATH_TRADE_AMOUNT)
	}
	if amount.Kind != spec.EFFECT_KIND_SET || amount.Value != 10 {
		t.Fatalf("spend amount = %+v, want set 10", amount)
	}
	result, ok := findChange(report.Changes, OP_SPEND, PATH_TRADE_RESULT)
	if !ok {
		t.Fatalf("missing %q spend row", PATH_TRADE_RESULT)
	}
	if result.Kind != spec.EFFECT_KIND_TEXT || result.Value != RESULT_SPENT {
		t.Fatalf("spend result = %+v, want text %q", result, RESULT_SPENT)
	}
	for _, change := range report.Changes {
		if change.Path == "交易.货币" {
			t.Fatalf("spend row targets the currency field: %+v", change)
		}
	}
}

// TestSettleFreeFlowHasNoSpendRow proves a zero cost is free and writes no
// trade request.
func TestSettleFreeFlowHasNoSpendRow(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Consume = nil
	flow.Produce = nil
	flow.Effects = nil
	deps, _, _ := testDeps(NewSnapshot(nil))

	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	if len(report.Changes) != 0 {
		t.Fatalf("changes = %d, want 0", len(report.Changes))
	}
}

// TestItemViewShapes proves the list shape and the name-keyed map shape both
// resolve the consumed item and yield the same decrement row.
func TestItemViewShapes(t *testing.T) {
	views := map[string]map[string]any{
		"list": {
			"素材.物品": []any{
				map[string]any{"名称": "食材", "所属场景": "S001-KT", "数量": 2},
			},
		},
		"name-keyed map": {
			"素材.物品": map[string]any{
				"食材": map[string]any{"所属场景": "S001-KT", "数量": 2},
			},
		},
	}
	for name, data := range views {
		t.Run(name, func(t *testing.T) {
			flow := baseFlow()
			flow.Cost = 0
			flow.Produce = nil
			flow.Effects = nil
			deps, _, _ := testDeps(NewSnapshot(data))

			_, report := Settle(flow, okVerdict(), doneState(flow), deps)
			if report.Failed() {
				t.Fatalf("Settle: %v", report.Errors)
			}
			consume, ok := findChange(report.Changes, OP_CONSUME, PATH_ITEM_QUANTITY)
			if !ok {
				t.Fatalf("missing consume row: %+v", report.Changes)
			}
			want := Change{Op: OP_CONSUME, Entity: "食材", Scene: "S001-KT", Path: PATH_ITEM_QUANTITY, Kind: spec.EFFECT_KIND_ADD, Value: -1}
			if changeKey(consume) != changeKey(want) {
				t.Fatalf("consume row = %+v, want %+v", consume, want)
			}
		})
	}
}

// TestConsumeUsesTemplateScene proves the same-name item in another scene is
// never consumed (C5) and that a short quantity refuses the whole settlement.
func TestConsumeUsesTemplateScene(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Produce = nil
	flow.Effects = nil

	deps, store, _ := testDeps(NewSnapshot(baseSnapshotData()))
	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	consume, ok := findChange(report.Changes, OP_CONSUME, PATH_ITEM_QUANTITY)
	if !ok {
		t.Fatalf("missing consume row: %+v", report.Changes)
	}
	if consume.Scene != "S001-KT" || consume.Value != -1 {
		t.Fatalf("consume row = %+v, want scene S001-KT value -1", consume)
	}

	short := map[string]any{"素材.物品": []any{
		map[string]any{"名称": "食材", "所属场景": "S999", "数量": 5},
	}}
	deps, store, ledger := testDeps(NewSnapshot(short))
	completion, refusedReport := Settle(flow, okVerdict(), doneState(flow), deps)
	assertRefused(t, flow, completion, refusedReport, store, ledger)
}

// TestProduceCarriesTemplate proves a produced item row carries its K1 template
// so the store can create a material entity with the corpus type and text.
func TestProduceCarriesTemplate(t *testing.T) {
	flow := baseFlow()
	flow.Cost = 0
	flow.Consume = nil
	flow.Effects = nil
	flow.Produce = []spec.ItemSpec{
		{Type: "素材", Name: "食物", Description: "热气腾腾的饭菜"},
		{Type: "素材", Name: "食物", Description: "热气腾腾的饭菜"},
	}
	deps, _, _ := testDeps(NewSnapshot(nil))

	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	produce, ok := findChange(report.Changes, OP_PRODUCE, PATH_ITEM_QUANTITY)
	if !ok {
		t.Fatalf("missing produce row: %+v", report.Changes)
	}
	if produce.Entity != "食物" || produce.Scene != "S001-KT" || produce.Value != 2 {
		t.Fatalf("produce row = %+v, want 食物/S001-KT/2", produce)
	}
	if produce.Item == nil || produce.Item.Type != "素材" || produce.Item.Description == "" {
		t.Fatalf("produce row lost its template: %+v", produce.Item)
	}
}

// changeKey compares two changes without the Item pointer, which tests compare
// field by field where it matters.
func changeKey(change Change) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%v", change.Op, change.Entity, change.Scene, change.Path, change.Kind, change.Value)
}
