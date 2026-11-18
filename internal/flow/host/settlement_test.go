// settlement_test.go pins the settlement seam behavior on a real store: effects
// land on their declared targets, a spend files a trade request without touching
// the balance (C5), and a refused batch restores every row it had written.
package host

import (
	"reflect"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestSettlementAppliesEffectsToTargets runs 吃饭 and pins both actor target rows
// plus the consumed material, before and after a reload.
func TestSettlementAppliesEffectsToTargets(t *testing.T) {
	store, dir := newTestStore(t, npcTemplate)
	actor := createActor(t, store)
	createMaterial(t, store, "食物@S001-KT", "食物", testScene, 1)

	hostStore := NewEntityStore(store)
	ledger := NewLedger()
	snapshot, err := hostStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	flow := eatingFlow()
	verdict := condition.Judge(flow, snapshot)
	if !verdict.OK {
		t.Fatalf("judge 吃饭: %+v", verdict.Reasons)
	}
	completion, report := settle.Settle(flow, verdict, runSteps(t, &flow, "exec-吃饭", 4), settle.Deps{
		Snapshot: snapshot,
		Store:    hostStore,
		Ledger:   ledger.SettlementLedger(),
		Entity:   testActorID,
		Tick:     4,
	})
	if report.Failed() {
		t.Fatalf("settle 吃饭: %v", report.Errors)
	}
	if completion.Result != settle.RESULT_COMPLETED {
		t.Fatalf("completion = %+v", completion)
	}
	requireInt(t, actor, settle.PATH_STOMACH_CONTENT, 80)
	requireBool(t, actor, "标记.饥饿", false)
	requireItemQuantity(t, store, "食物", testScene, 0)

	if err := hostStore.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	reloaded := reopenStore(t, dir).Get(testActorID)
	if reloaded == nil {
		t.Fatal("actor missing after reload")
	}
	requireInt(t, reloaded, settle.PATH_STOMACH_CONTENT, 80)
	requireBool(t, reloaded, "标记.饥饿", false)
}

// TestSettlementSpendFilesTradeRequest proves settlement only files the trade
// request: 成交额/结果 are written, while 交易.货币 stays with the trade tick (C5).
func TestSettlementSpendFilesTradeRequest(t *testing.T) {
	store, dir := newTestStore(t, npcTemplate)
	actor := createActor(t, store)
	if err := actor.Set(condition.PATH_SCENE, testOtherScene); err != nil {
		t.Fatalf("move actor: %v", err)
	}
	createMaterial(t, store, "食材@S008", "食材", testOtherScene, 2)

	hostStore := NewEntityStore(store)
	ledger := NewLedger()
	snapshot, err := hostStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	flow := buyingFlow()
	verdict := condition.Judge(flow, snapshot)
	if !verdict.OK {
		t.Fatalf("judge 购买食材: %+v", verdict.Reasons)
	}
	_, report := settle.Settle(flow, verdict, runSteps(t, &flow, "exec-购买", 5), settle.Deps{
		Snapshot: snapshot,
		Store:    hostStore,
		Ledger:   ledger.SettlementLedger(),
		Entity:   testActorID,
		Tick:     5,
	})
	if report.Failed() {
		t.Fatalf("settle 购买食材: %v", report.Errors)
	}
	requireInt(t, actor, settle.PATH_TRADE_AMOUNT, 10)
	requireText(t, actor, settle.PATH_TRADE_RESULT, settle.RESULT_SPENT)
	requireInt(t, actor, condition.PATH_MONEY, 100)
	requireItemQuantity(t, store, "食材", testOtherScene, 3)

	if err := hostStore.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	reloaded := reopenStore(t, dir).Get(testActorID)
	if reloaded == nil {
		t.Fatal("actor missing after reload")
	}
	requireInt(t, reloaded, settle.PATH_TRADE_AMOUNT, 10)
	requireText(t, reloaded, settle.PATH_TRADE_RESULT, settle.RESULT_SPENT)
	requireInt(t, reloaded, condition.PATH_MONEY, 100)
}

// TestSettlementFailureLeavesEntityUntouched marks the second effect target
// readonly so commit writes the first rows and is then rejected; the rollback
// must restore every row across both entities.
func TestSettlementFailureLeavesEntityUntouched(t *testing.T) {
	store, _ := newTestStore(t, readonlyNPCTemplate)
	actor := createActor(t, store)
	createMaterial(t, store, "食物@S001-KT", "食物", testScene, 2)

	hostStore := NewEntityStore(store)
	ledger := NewLedger()
	snapshot, err := hostStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	flow := eatingFlow()
	verdict := condition.Judge(flow, snapshot)
	if !verdict.OK {
		t.Fatalf("judge 吃饭: %+v", verdict.Reasons)
	}
	before := actor.Snapshot()
	completion, report := settle.Settle(flow, verdict, runSteps(t, &flow, "exec-吃饭", 4), settle.Deps{
		Snapshot: snapshot,
		Store:    hostStore,
		Ledger:   ledger.SettlementLedger(),
		Entity:   testActorID,
		Tick:     4,
	})
	if !report.Failed() {
		t.Fatal("settle 吃饭 on a readonly target was not refused")
	}
	if completion != (settle.Completion{}) {
		t.Fatalf("refused settlement produced completion %+v", completion)
	}
	if !strings.Contains(report.Errors[0].Error(), "标记.饥饿") {
		t.Fatalf("refusal does not name the readonly row: %v", report.Errors)
	}
	after := actor.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("actor data partially modified:\nbefore=%#v\nafter=%#v", before, after)
	}
	requireItemQuantity(t, store, "食物", testScene, 2)
	if _, ok := ledger.SettlementLedger().Lookup(flow.ID, testActorID); ok {
		t.Fatal("refused settlement was recorded in the ledger")
	}
}

// TestApplyRefusesUnknownChange keeps a hand-built batch from creating data: an
// unknown actor or operation is refused before any write.
func TestApplyRefusesUnknownChange(t *testing.T) {
	store, _ := newTestStore(t, npcTemplate)
	createActor(t, store)
	hostStore := NewEntityStore(store)

	missing := []settle.Change{{
		Op: settle.OP_EFFECT, Entity: "不存在", Path: "代谢.胃内容",
		Kind: spec.EFFECT_KIND_SET, Value: 1,
	}}
	if err := hostStore.Apply("confused", missing); err == nil {
		t.Fatal("Apply accepted a change on a missing entity")
	}
	unknown := []settle.Change{{Op: settle.ChangeOp("wipe"), Entity: testActorID}}
	if err := hostStore.Apply("confused", unknown); err == nil {
		t.Fatal("Apply accepted an unknown ChangeOp")
	}
}
