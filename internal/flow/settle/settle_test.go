// settle_test.go covers the K4 happy path, the one-settlement invariant and the
// zero-write refusal contract, including a broken row after valid ones.
package settle

import (
	"errors"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
)

// TestSingleSettlement is m4's idempotency test: settling the same
// (flowID, entity) twice returns the recorded completion, applies no second
// write set and reports no changes.
func TestSingleSettlement(t *testing.T) {
	flow := baseFlow()
	deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))

	first, firstReport := Settle(flow, okVerdict(), doneState(flow), deps)
	if firstReport.Failed() {
		t.Fatalf("first Settle: %v", firstReport.Errors)
	}
	rowsAfterFirst := store.writes()
	if rowsAfterFirst != len(firstReport.Changes) {
		t.Fatalf("store rows = %d, report changes = %d", rowsAfterFirst, len(firstReport.Changes))
	}

	second, secondReport := Settle(flow, okVerdict(), doneState(flow), deps)
	if secondReport.Failed() {
		t.Fatalf("second Settle: %v", secondReport.Errors)
	}
	if second != first {
		t.Fatalf("replay Completion = %+v, want %+v", second, first)
	}
	if len(secondReport.Changes) != 0 {
		t.Fatalf("replay changes = %d, want 0", len(secondReport.Changes))
	}
	if store.calls() != 1 {
		t.Fatalf("store Apply calls = %d, want 1", store.calls())
	}
	if store.writes() != rowsAfterFirst {
		t.Fatalf("store rows after replay = %d, want %d", store.writes(), rowsAfterFirst)
	}
	if recorded, ok := ledger.Lookup(flow.ID, testEntity); !ok || recorded != first {
		t.Fatalf("ledger entry = %+v, ok = %v", recorded, ok)
	}
}

// TestSettleRefusalLeavesNoWrites proves a refused settlement produces no
// Completion, no store call and no ledger entry. The valid-part-then-invalid
// case is the atomicity check: planning must reject before any write.
func TestSettleRefusalLeavesNoWrites(t *testing.T) {
	cases := map[string]func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger){
		"later effect is unknown": func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
			flow := baseFlow()
			flow.Effects = append(flow.Effects, spec.EffectSpec{
				Path:         "未知.字段",
				Kind:         spec.EFFECT_KIND_SET,
				Value:        1,
				Target:       sourceTarget("未知.字段"),
				TargetReason: "未在 D6 映射表登记",
			})
			return flowWithSnapshot(t, flow, baseSnapshotData())
		},
		"effect literal contradicts kind": func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
			flow := baseFlow()
			flow.Effects = []spec.EffectSpec{{
				Path:         "成长.经验",
				Kind:         spec.EFFECT_KIND_SET,
				Value:        "not-a-number",
				Target:       sourceTarget("成长.经验"),
				TargetReason: "同名字段",
			}}
			return flowWithSnapshot(t, flow, baseSnapshotData())
		},
		"consume item missing": func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
			return flowWithSnapshot(t, baseFlow(), map[string]any{})
		},
		"consume item too short": func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
			view := map[string]any{"素材.物品": []any{
				map[string]any{"名称": "食材", "所属场景": "S001-KT", "数量": 0},
			}}
			return flowWithSnapshot(t, baseFlow(), view)
		},
		"negative cost": func(t *testing.T) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
			flow := baseFlow()
			flow.Cost = -1
			return flowWithSnapshot(t, flow, baseSnapshotData())
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			flow, snap, store, ledger := build(t)
			deps := Deps{Snapshot: snap, Store: store, Ledger: ledger, Entity: testEntity, Tick: 9}
			completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
			assertRefused(t, flow, completion, report, store, ledger)
		})
	}
}

// TestSettleStoreFailureIsAtomic proves a store error is reported as a
// settlement failure with no Completion. The fake store persists nothing when
// it fails, matching the atomic Store contract.
func TestSettleStoreFailureIsAtomic(t *testing.T) {
	flow := baseFlow()
	deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))
	store.fail = errors.New("disk full")

	completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if !report.Failed() {
		t.Fatalf("Settle accepted a failing store")
	}
	if completion != (Completion{}) {
		t.Fatalf("Completion = %+v, want zero", completion)
	}
	if len(report.Changes) != 0 {
		t.Fatalf("report changes = %d, want 0", len(report.Changes))
	}
	if store.calls() != 1 {
		t.Fatalf("store Apply calls = %d, want 1", store.calls())
	}
	if store.writes() != 0 {
		t.Fatalf("store writes = %d, want 0", store.writes())
	}
	if _, ok := ledger.Lookup(flow.ID, testEntity); ok {
		t.Fatalf("ledger recorded a failed settlement")
	}
	if !strings.Contains(report.Errors[0].Error(), "disk full") {
		t.Fatalf("error %q does not carry the store failure", report.Errors[0])
	}
}

// TestSettlePreconditions proves K2/K3/dependency refusals happen before any
// planning or write.
func TestSettlePreconditions(t *testing.T) {
	cases := map[string]func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps){
		"verdict not ok": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			verdict := condition.Verdict{OK: false, Reasons: []condition.Reason{{
				Code: condition.REASON_MISSING_ITEM, Path: condition.PATH_ITEMS, Want: "1", Got: "0",
			}}}
			return flow, verdict, doneState(flow), deps
		},
		"state not done": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			state := doneState(flow)
			state.Status = step.StatusRunning
			return flow, okVerdict(), state, deps
		},
		"state belongs to another flow": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			state := doneState(flow)
			state.FlowID = "别的流程"
			return flow, okVerdict(), state, deps
		},
		"empty entity": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			deps.Entity = ""
			return flow, okVerdict(), doneState(flow), deps
		},
		"empty flow id": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			flow.ID = ""
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			return flow, okVerdict(), doneState(flow), deps
		},
		"nil snapshot": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(nil)
			return flow, okVerdict(), doneState(flow), deps
		},
		"nil store": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			deps.Store = nil
			return flow, okVerdict(), doneState(flow), deps
		},
		"nil ledger": func(t *testing.T) (spec.FlowSpec, condition.Verdict, step.State, Deps) {
			flow := baseFlow()
			deps, _, _ := testDeps(NewSnapshot(baseSnapshotData()))
			deps.Ledger = nil
			return flow, okVerdict(), doneState(flow), deps
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			flow, verdict, state, deps := build(t)
			store, _ := deps.Store.(*fakeStore)
			ledger, _ := deps.Ledger.(*MemoryLedger)
			completion, report := Settle(flow, verdict, state, deps)
			if !report.Failed() {
				t.Fatalf("Settle accepted a broken precondition for flow %q", flow.ID)
			}
			if completion != (Completion{}) {
				t.Fatalf("Completion = %+v, want zero", completion)
			}
			if store != nil && (store.calls() != 0 || store.writes() != 0) {
				t.Fatalf("store calls = %d, writes = %d, want 0/0", store.calls(), store.writes())
			}
			if ledger != nil {
				if _, ok := ledger.Lookup(flow.ID, testEntity); ok {
					t.Fatalf("ledger recorded a refused settlement")
				}
			}
		})
	}
}

// flowWithSnapshot builds one settlement setup from a flow and raw entity map.
func flowWithSnapshot(t *testing.T, flow spec.FlowSpec, data map[string]any) (spec.FlowSpec, condition.Snapshot, *fakeStore, *MemoryLedger) {
	t.Helper()
	deps, store, ledger := testDeps(NewSnapshot(data))
	return flow, deps.Snapshot, store, ledger
}

// assertRefused asserts the K4 failure shape: one or more report errors, no
// Completion, zero store calls/writes and no ledger record.
func assertRefused(t *testing.T, flow spec.FlowSpec, completion Completion, report Report, store *fakeStore, ledger *MemoryLedger) {
	t.Helper()
	if !report.Failed() {
		t.Fatalf("flow %q was accepted, want refusal", flow.ID)
	}
	if completion != (Completion{}) {
		t.Fatalf("Completion = %+v, want zero", completion)
	}
	if store.calls() != 0 || store.writes() != 0 {
		t.Fatalf("store calls = %d, writes = %d, want 0/0", store.calls(), store.writes())
	}
	if len(report.Changes) != 0 {
		t.Fatalf("report changes = %d, want 0", len(report.Changes))
	}
	if _, ok := ledger.Lookup(flow.ID, testEntity); ok {
		t.Fatalf("ledger recorded a refused settlement")
	}
}
