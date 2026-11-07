// ops_test.go covers the shared operations: dependency validation, nightly
// decay, judging, the candidate queue and the repair rejection paths.
package ops_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/control/ops"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/expert"
)

// newDeps assembles a temp-dir engine with one NPC entity.
func newDeps(t *testing.T) ops.Deps {
	t.Helper()
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Create("猫猫", "NPC", map[string]any{"状态": map[string]any{"情绪": "平静"}}, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	return ops.Deps{Store: store, Machine: engine.New(store), DataDir: dataDir}
}

// requireKind asserts that err is a classified ops failure of the given kind.
func requireKind(t *testing.T, err error, kind ops.Kind) *ops.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s failure, got nil", kind)
	}
	var failure *ops.Error
	if !errors.As(err, &failure) {
		t.Fatalf("error %v is not a classified ops failure", err)
	}
	if failure.Kind != kind {
		t.Fatalf("kind = %s, want %s (%v)", failure.Kind, kind, err)
	}
	return failure
}

// TestZeroDepsFailsActionably covers a connection that was never wired: every
// operation reports an internal configuration failure instead of panicking.
func TestZeroDepsFailsActionably(t *testing.T) {
	_, err := ops.Nightly(ops.Deps{}, nil)
	requireKind(t, err, ops.KIND_INTERNAL)
	_, err = ops.Candidates(ops.Deps{})
	requireKind(t, err, ops.KIND_INTERNAL)
	_, err = ops.RollbackRepair(ops.Deps{})
	requireKind(t, err, ops.KIND_INTERNAL)
}

// TestNightlyDecaysAndPersists covers a successful decay: the weight drop is
// reported per expert and the result is written to experts.json.
func TestNightlyDecaysAndPersists(t *testing.T) {
	deps := newDeps(t)
	deps.Experts = expert.NewDefaultRegistry(expert.DEFAULT_CONTEXT_CAPACITY)
	deps.Experts.Remember("事件记忆", expert.Entry{SnapshotID: "evt_1", Content: "旧事", Weight: 0.06})
	factor := 0.5
	body, err := ops.Nightly(deps, &factor)
	if err != nil {
		t.Fatalf("nightly: %v", err)
	}
	if body["ok"] != true || body["factor"] != 0.5 {
		t.Fatalf("body = %v", body)
	}
	if body["removed"] != 1 {
		t.Fatalf("removed = %v, want 1", body["removed"])
	}
	if _, err := os.Stat(filepath.Join(deps.DataDir, "experts.json")); err != nil {
		t.Fatalf("experts.json not persisted: %v", err)
	}
	if _, err := ops.Nightly(deps, float64Ptr(1.5)); err == nil {
		t.Fatal("factor 1.5 must be rejected")
	} else {
		requireKind(t, err, ops.KIND_BAD_REQUEST)
	}
}

// TestJudgeQueuesUncovered covers the judge path: an uncovered candidate is
// queued with an ID, then listed and resolved as approved.
func TestJudgeQueuesUncovered(t *testing.T) {
	deps := newDeps(t)
	body, err := ops.Judge(deps, ops.JudgeRequest{EntityID: "猫猫", Path: "状态.信任值", Proposed: 0.3, Source: "teacher"})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if body["queued"] != true {
		t.Fatalf("queued = %v, want true", body["queued"])
	}
	id, _ := body["candidate_id"].(string)
	if id == "" {
		t.Fatalf("candidate_id missing: %v", body)
	}
	listed, err := ops.Candidates(deps)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if listed["count"] != 1 {
		t.Fatalf("count = %v, want 1", listed["count"])
	}
	resolved, err := ops.ResolveCandidate(deps, ops.ResolveRequest{ID: id, Status: candidates.STATUS_APPROVED, Note: "人工确认"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved["ok"] != true {
		t.Fatalf("resolve body = %v", resolved)
	}
	after, err := ops.Candidates(deps)
	if err != nil || after["count"] != 0 {
		t.Fatalf("pending after resolve = %v (%v)", after["count"], err)
	}
}

// TestJudgeRejectsBadRequests covers the required judge fields.
func TestJudgeRejectsBadRequests(t *testing.T) {
	deps := newDeps(t)
	_, err := ops.Judge(deps, ops.JudgeRequest{Path: "状态.情绪"})
	requireKind(t, err, ops.KIND_BAD_REQUEST)
	_, err = ops.Judge(deps, ops.JudgeRequest{EntityID: "猫猫"})
	requireKind(t, err, ops.KIND_BAD_REQUEST)
	_, err = ops.ResolveCandidate(deps, ops.ResolveRequest{ID: "x", Status: "bad"})
	requireKind(t, err, ops.KIND_BAD_REQUEST)
	_, err = ops.ResolveCandidate(deps, ops.ResolveRequest{ID: "missing", Status: candidates.STATUS_APPROVED})
	requireKind(t, err, ops.KIND_NOT_FOUND)
}

// TestApplyCandidateRejections covers the promotion guards: unknown/pending
// candidates, missing snapshot anchor and missing when condition.
func TestApplyCandidateRejections(t *testing.T) {
	deps := newDeps(t)
	_, err := ops.ApplyCandidate(deps, ops.ApplyRequest{})
	requireKind(t, err, ops.KIND_BAD_REQUEST)
	_, err = ops.ApplyCandidate(deps, ops.ApplyRequest{ID: "cand_missing"})
	requireKind(t, err, ops.KIND_NOT_FOUND)

	store := candidates.NewStore(filepath.Join(deps.DataDir, "candidates.jsonl"))
	pending, err := store.Append(candidates.Record{EntityID: "猫猫", Path: "状态.信任值", Proposed: 0.3})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	_, err = ops.ApplyCandidate(deps, ops.ApplyRequest{ID: pending.ID, When: "1 == 1"})
	requireKind(t, err, ops.KIND_BAD_REQUEST)

	approved, err := store.Resolve(pending.ID, candidates.STATUS_APPROVED, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	_, err = ops.ApplyCandidate(deps, ops.ApplyRequest{ID: approved.ID, When: "1 == 1"})
	requireKind(t, err, ops.KIND_BAD_REQUEST)

	withSnapshot, err := store.Append(candidates.Record{
		EntityID: "猫猫", Path: "状态.信任值", Proposed: 0.3,
		Evidence: []string{"snapshot:evt_1"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := store.Resolve(withSnapshot.ID, candidates.STATUS_APPROVED, ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	_, err = ops.ApplyCandidate(deps, ops.ApplyRequest{ID: withSnapshot.ID})
	requireKind(t, err, ops.KIND_BAD_REQUEST)
}

// TestRollbackRepairWithoutVersions covers the empty repair history.
func TestRollbackRepairWithoutVersions(t *testing.T) {
	deps := newDeps(t)
	_, err := ops.RollbackRepair(deps)
	requireKind(t, err, ops.KIND_INTERNAL)
}

// float64Ptr returns a pointer to value for the optional factor argument.
func float64Ptr(value float64) *float64 { return &value }
