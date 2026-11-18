// flowChain_test.go is the real-store integration walk: judge → step → settle
// over one entity.Store rooted in a temp directory, with persistence checked by
// reloading a second store from the same directory.
package host

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
)

// TestFlowChainThroughEntityStore settles 做饭 on a real entity store and proves
// the rows reach the disk only through Flush. Settlement writes an interaction
// intent and a produced item; it never mutates the consumed material.
func TestFlowChainThroughEntityStore(t *testing.T) {
	store, dir := newTestStore(t, npcTemplate)
	createActor(t, store)
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
	if len(report.Changes) != 3 {
		t.Fatalf("changes = %d, want 3 (2 interaction + 1 produce): %+v", len(report.Changes), report.Changes)
	}
	requireItemQuantity(t, store, "食材", testScene, 1)
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

	// Reopening the ledger proves the settlement record survived.
	if _, err := OpenLedger(ledgerPath); err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	reopened, err := OpenLedger(ledgerPath)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	if recorded, ok := reopened.SettlementLedger().Lookup("做饭", testActorID); !ok || recorded != completion {
		t.Fatalf("reopened settlement = %+v, ok = %v", recorded, ok)
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
}
