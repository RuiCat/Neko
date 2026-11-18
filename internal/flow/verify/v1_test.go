// v1_test.go is the V1 acceptance walk: the artifact flows 购买食材 → 做饭 →
// 吃饭 settle in order against one in-memory entity, and the resulting money,
// material and metabolism rows are asserted.
package verify

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
)

// TestFlowChain walks the three frozen flows and pins the post-chain entity
// state: 10 currency spent, the purchase material created at its scene, the
// kitchen ingredient consumed, the cooked food consumed, 代谢.胃内容 = 1200
// (the legacy 80 satiety scaled by 15) and 标记.饥饿 = false.
func TestFlowChain(t *testing.T) {
	store := newFixtureStore()
	ledger := settle.NewMemoryLedger()
	completions := runFlowChain(t, store, ledger)
	if len(completions) != 3 {
		t.Fatalf("completions = %d, want 3", len(completions))
	}

	requireInt(t, store, condition.PATH_MONEY, 90)
	requireInt(t, store, settle.PATH_TRADE_AMOUNT, 10)
	requireText(t, store, settle.PATH_TRADE_RESULT, settle.RESULT_SPENT)
	requireItemQuantity(t, store, "食材", "S008", 1)
	requireItemQuantity(t, store, "食材", "S001-KT", 0)
	requireItemQuantity(t, store, "食物", "S001-KT", 0)
	requireText(t, store, condition.PATH_SCENE, "S001-KT")
	requireInt(t, store, settle.PATH_STOMACH_CONTENT, 1200)
	requireBool(t, store, PATH_HUNGER, false)

	if recorded, ok := ledger.Lookup("吃饭", TEST_ENTITY); !ok || recorded != completions[2] {
		t.Fatalf("ledger entry for 吃饭 = %+v, ok = %v", recorded, ok)
	}
	t.Logf("chain ok: 交易.货币=90 成交额=10 结果=%s 食材@S008=1 代谢.胃内容=1200 标记.饥饿=false",
		settle.RESULT_SPENT)
}

// TestFlowChainIntermediateState pins the same chain after the purchase and after
// cooking, so a broken middle step cannot be hidden by the final totals.
func TestFlowChainIntermediateState(t *testing.T) {
	document := flowSpecFixture(t)
	store := newFixtureStore()
	ledger := settle.NewMemoryLedger()

	runFlow(t, store, ledger, flowByID(t, document, "购买食材"))
	requireInt(t, store, condition.PATH_MONEY, 90)
	requireItemQuantity(t, store, "食材", "S008", 1)
	requireItemQuantity(t, store, "食材", "S001-KT", 1)
	requireInt(t, store, settle.PATH_STOMACH_CONTENT, 300)

	runFlow(t, store, ledger, flowByID(t, document, "做饭"))
	requireItemQuantity(t, store, "食材", "S001-KT", 0)
	requireItemQuantity(t, store, "食物", "S001-KT", 1)
	requireInt(t, store, settle.PATH_STOMACH_CONTENT, 300)

	runFlow(t, store, ledger, flowByID(t, document, "吃饭"))
	requireItemQuantity(t, store, "食物", "S001-KT", 0)
	requireInt(t, store, settle.PATH_STOMACH_CONTENT, 1200)
	requireBool(t, store, PATH_HUNGER, false)
}
