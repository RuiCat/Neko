// flowChain_test.go is the real-store integration walk: judge → step → settle →
// task over one entity.Store rooted in a temp directory, with persistence checked
// by reloading a second store from the same directory.
package host

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/task"
)

// TestFlowChainThroughEntityStore settles 做饭 on a real entity store, advances
// the matching task, and proves the rows reach the disk only through Flush.
func TestFlowChainThroughEntityStore(t *testing.T) {
	store, dir := newTestStore(t, npcTemplate)
	actor := createActor(t, store)
	createMaterial(t, store, "食材@S001-KT", "食材", testScene, 1)
	createMaterial(t, store, "食材@S008", "食材", testOtherScene, 2)

	hostStore := NewEntityStore(store)
	if err := hostStore.Flush(); err != nil {
		t.Fatalf("flush baseline: %v", err)
	}

	ledgerPath := filepath.Join(dir, "ledger.json")
	ledger, err := OpenLedger(ledgerPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	snapshot, err := hostStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	flow := cookingFlow()
	verdict := condition.Judge(flow, snapshot)
	if !verdict.OK {
		t.Fatalf("judge 做饭: %+v", verdict.Reasons)
	}
	state := runSteps(t, &flow, "exec-做饭", 7)
	completion, report := settle.Settle(flow, verdict, state, settle.Deps{
		Snapshot: snapshot,
		Store:    hostStore,
		Ledger:   ledger.SettlementLedger(),
		Entity:   testActorID,
		Tick:     7,
	})
	if report.Failed() {
		t.Fatalf("settle 做饭: %v", report.Errors)
	}
	wantCompletion := settle.Completion{
		FlowID: "做饭", Entity: testActorID, Result: settle.RESULT_COMPLETED, Tick: 7,
	}
	if completion != wantCompletion {
		t.Fatalf("completion = %+v, want %+v", completion, wantCompletion)
	}
	if len(report.Changes) != 2 {
		t.Fatalf("changes = %d, want 2 (consume + produce)", len(report.Changes))
	}
	requireItemQuantity(t, store, "食材", testScene, 0)
	requireItemQuantity(t, store, "食材", testOtherScene, 2)
	requireItemQuantity(t, store, "食物", testScene, 1)
	if created := materialEntity(t, store, "食物", testScene); created.Type != MATERIAL_TYPE {
		t.Fatalf("produced material type = %q, want %q", created.Type, MATERIAL_TYPE)
	}

	// Settlement writes memory only: a reload before Flush shows no 食物.
	beforeFlush := reopenStore(t, dir)
	requireItemQuantity(t, beforeFlush, "食物", testScene, 0)
	if err := hostStore.Flush(); err != nil {
		t.Fatalf("flush after settle: %v", err)
	}
	reloaded := reopenStore(t, dir)
	requireItemQuantity(t, reloaded, "食材", testScene, 0)
	requireItemQuantity(t, reloaded, "食材", testOtherScene, 2)
	requireItemQuantity(t, reloaded, "食物", testScene, 1)

	// The reloaded (JSON-decoded) material view still judges.
	reloadedStore := NewEntityStore(reloaded)
	reloadedSnapshot, err := reloadedStore.Snapshot(testActorID)
	if err != nil {
		t.Fatalf("reloaded snapshot: %v", err)
	}
	if verdict := condition.Judge(eatingFlow(), reloadedSnapshot); !verdict.OK {
		t.Fatalf("judge 吃饭 on reloaded store: %+v", verdict.Reasons)
	}

	templates := []spec.TaskTemplateSpec{cookingTask()}
	transition, err := task.Advance(completion, templates, task.Deps{
		Store: hostStore, Ledger: ledger.TaskLedger(),
	})
	if err != nil {
		t.Fatalf("advance 做饭: %v", err)
	}
	if transition.Replay {
		t.Fatal("first advance reported itself as a replay")
	}
	if transition.FromStage != task.STAGE_INACTIVE || transition.ToStage != task.STAGE_COMPLETED {
		t.Fatalf("transition %s -> %s", transition.FromStage, transition.ToStage)
	}
	if !reflect.DeepEqual(transition.Stages, []string{task.STAGE_ACTIVE, task.STAGE_COMPLETED}) {
		t.Fatalf("stages = %v", transition.Stages)
	}
	requireText(t, actor, task.PATH_TASK_CURRENT, "做饭")
	requireInt(t, actor, task.PATH_TASK_PROGRESS, task.PROGRESS_COMPLETE)
	requireInt(t, actor, task.PATH_GROWTH_EXPERIENCE, 20)
	requireInt(t, actor, task.PATH_RELATION_AFFINITY, 5)

	if err := hostStore.Flush(); err != nil {
		t.Fatalf("flush after advance: %v", err)
	}
	reloadedActor := reopenStore(t, dir).Get(testActorID)
	if reloadedActor == nil {
		t.Fatal("actor missing after reload")
	}
	requireText(t, reloadedActor, task.PATH_TASK_CURRENT, "做饭")
	requireInt(t, reloadedActor, task.PATH_TASK_PROGRESS, task.PROGRESS_COMPLETE)
	requireInt(t, reloadedActor, task.PATH_GROWTH_EXPERIENCE, 20)
	requireInt(t, reloadedActor, task.PATH_RELATION_AFFINITY, 5)

	// Reopening the ledger proves both records survived the process boundary.
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatalf("ledger file: %v", err)
	}
	reopened, err := OpenLedger(ledgerPath)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	if err := reopened.PersistError(); err != nil {
		t.Fatalf("ledger persistence: %v", err)
	}
	if recorded, ok := reopened.SettlementLedger().Lookup("做饭", testActorID); !ok || recorded != completion {
		t.Fatalf("reopened settlement = %+v, ok = %v", recorded, ok)
	}
	if recorded, ok := reopened.TaskLedger().Lookup("做饭", testActorID); !ok || recorded.ToStage != task.STAGE_COMPLETED {
		t.Fatalf("reopened advance = %+v, ok = %v", recorded, ok)
	}

	// Settlement replay through the reopened ledger writes nothing again.
	replayedCompletion, replayReport := settle.Settle(flow, verdict, state, settle.Deps{
		Snapshot: snapshot,
		Store:    hostStore,
		Ledger:   reopened.SettlementLedger(),
		Entity:   testActorID,
		Tick:     7,
	})
	if replayReport.Failed() {
		t.Fatalf("replay settle: %v", replayReport.Errors)
	}
	if replayedCompletion != completion {
		t.Fatalf("replayed completion = %+v, want %+v", replayedCompletion, completion)
	}
	if len(replayReport.Changes) != 0 {
		t.Fatalf("replay changes = %d, want 0", len(replayReport.Changes))
	}
	requireItemQuantity(t, store, "食物", testScene, 1)

	// Task replay through the reopened ledger pays no second reward.
	replayedTransition, err := task.Advance(completion, templates, task.Deps{
		Store: hostStore, Ledger: reopened.TaskLedger(),
	})
	if err != nil {
		t.Fatalf("replay advance: %v", err)
	}
	if !replayedTransition.Replay || len(replayedTransition.Rewards) != 0 {
		t.Fatalf("replay advance = %+v", replayedTransition)
	}
	requireInt(t, actor, task.PATH_GROWTH_EXPERIENCE, 20)
}

// TestTaskAdvanceStoreBackedReplay covers the other half of m5's single
// settlement: with the rules having moved 任务.阶段 to 完成, an empty ledger still
// reports a replay and pays nothing.
func TestTaskAdvanceStoreBackedReplay(t *testing.T) {
	store, _ := newTestStore(t, npcTemplate)
	actor := createActor(t, store)
	hostStore := NewEntityStore(store)
	completion := settle.Completion{
		FlowID: "做饭", Entity: testActorID, Result: settle.RESULT_COMPLETED, Tick: 3,
	}
	templates := []spec.TaskTemplateSpec{cookingTask()}

	if _, err := task.Advance(completion, templates, task.Deps{
		Store: hostStore, Ledger: NewLedger().TaskLedger(),
	}); err != nil {
		t.Fatalf("first advance: %v", err)
	}
	requireInt(t, actor, task.PATH_GROWTH_EXPERIENCE, 20)

	// The task rules own the stage row; the host only reads it.
	if err := actor.Set(task.PATH_TASK_STAGE, task.STAGE_COMPLETED); err != nil {
		t.Fatalf("simulate rules stage row: %v", err)
	}
	replayed, err := task.Advance(completion, templates, task.Deps{
		Store: hostStore, Ledger: NewLedger().TaskLedger(),
	})
	if err != nil {
		t.Fatalf("store-backed replay: %v", err)
	}
	if !replayed.Replay || replayed.FromStage != task.STAGE_COMPLETED || replayed.ToStage != task.STAGE_COMPLETED {
		t.Fatalf("store-backed replay = %+v", replayed)
	}
	requireInt(t, actor, task.PATH_GROWTH_EXPERIENCE, 20)
}
