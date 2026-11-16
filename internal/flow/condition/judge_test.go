// judge_test.go covers the six K2 rejection classes, the pass path's zero-write
// guarantee, replay stability and the item/scene resolution rules.
package condition

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// recordingSnapshot is a snapshot fake that also counts writes, so a test can
// prove judgment never mutates the view it was given.
type recordingSnapshot struct {
	data     map[string]any
	writes   int
	getCalls int
}

// Get reads one path from the fake view.
func (s *recordingSnapshot) Get(path string) (any, error) {
	s.getCalls++
	value, ok := s.data[path]
	if !ok {
		return nil, fmt.Errorf("snapshot path %q not found", path)
	}
	return value, nil
}

// Set exists only so a test can observe an unexpected write.
func (s *recordingSnapshot) Set(path string, value any) {
	s.writes++
	s.data[path] = value
}

// fingerprint renders the whole view deterministically for an unchanged check.
func (s *recordingSnapshot) fingerprint(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(s.data)
	if err != nil {
		t.Fatalf("marshal snapshot data: %v", err)
	}
	return string(encoded)
}

// itemRow builds one item-view record.
func itemRow(name, scene string, quantity int) map[string]any {
	return map[string]any{
		KEY_ITEM_NAME:     name,
		KEY_ITEM_SCENE:    scene,
		KEY_ITEM_QUANTITY: quantity,
	}
}

// assertSingleReason checks that the verdict rejects with exactly one reason.
func assertSingleReason(t *testing.T, verdict Verdict, want Reason) {
	t.Helper()
	if verdict.OK {
		t.Fatalf("verdict.OK = true, want false; reasons = %+v", verdict.Reasons)
	}
	if len(verdict.Reasons) != 1 {
		t.Fatalf("reasons = %+v, want exactly 1 reason", verdict.Reasons)
	}
	if verdict.Reasons[0] != want {
		t.Fatalf("reason = %+v, want %+v", verdict.Reasons[0], want)
	}
}

// assertUntouched checks the fake saw no write and the view is byte-identical.
func assertUntouched(t *testing.T, snap *recordingSnapshot, before string) {
	t.Helper()
	if snap.writes != 0 {
		t.Fatalf("judgment wrote through the snapshot view %d times", snap.writes)
	}
	if after := snap.fingerprint(t); after != before {
		t.Fatalf("snapshot changed:\n before = %s\n after  = %s", before, after)
	}
}

func TestJudgeRejectsMissingItem(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_SCENE: "S001-KT",
		PATH_ITEMS: []any{itemRow("食材", "S001-KT", 0)},
	}}
	before := snap.fingerprint(t)
	verdict := Judge(spec.FlowSpec{Scene: "S001-KT", Requires: []string{"食材"}}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_MISSING_ITEM,
		Path: PATH_ITEMS + ".食材",
		Want: "1",
		Got:  "0",
	})
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsMissingMoney(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{PATH_MONEY: 3}}
	before := snap.fingerprint(t)
	verdict := Judge(spec.FlowSpec{Cost: 10}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_MISSING_MONEY,
		Path: PATH_MONEY,
		Want: "10",
		Got:  "3",
	})
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsWrongScene(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{PATH_SCENE: "S008"}}
	before := snap.fingerprint(t)
	verdict := Judge(spec.FlowSpec{Scene: "S001-KT"}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_WRONG_SCENE,
		Path: PATH_SCENE,
		Want: "S001-KT",
		Got:  "S008",
	})
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsLowAffinity(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{PATH_AFFINITY: 30}}
	before := snap.fingerprint(t)
	verdict := JudgeRequirements([]Requirement{{Kind: KIND_AFFINITY, Threshold: 60}}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_LOW_AFFINITY,
		Path: PATH_AFFINITY,
		Want: "60",
		Got:  "30",
	})
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsNotReadyTime(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{}}
	before := snap.fingerprint(t)
	verdict := JudgeRequirements([]Requirement{{Kind: KIND_TIME, Threshold: 3}}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_NOT_READY,
		Path: PATH_TIME,
		Want: "day>=3",
		Got:  "",
	})
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsEvalError(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{PATH_MONEY: "lots"}}
	before := snap.fingerprint(t)
	verdict := Judge(spec.FlowSpec{Cost: 10}, snap)
	if verdict.OK || len(verdict.Reasons) != 1 {
		t.Fatalf("verdict = %+v, want one eval_error", verdict)
	}
	reason := verdict.Reasons[0]
	if reason.Code != REASON_EVAL_ERROR || reason.Path != PATH_MONEY {
		t.Fatalf("reason = %+v, want eval_error at %s", reason, PATH_MONEY)
	}
	assertUntouched(t, snap, before)
}

func TestJudgeRejectsUnknownRequirementKind(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{}}
	verdict := JudgeRequirements([]Requirement{{Kind: Kind("unknown")}}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_EVAL_ERROR,
		Path: "unknown",
		Want: "known requirement kind",
		Got:  "unknown",
	})
}

func TestJudgeRejectsNegativeCost(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{}}
	verdict := Judge(spec.FlowSpec{Cost: -1}, snap)
	assertSingleReason(t, verdict, Reason{
		Code: REASON_EVAL_ERROR,
		Path: "cost",
		Want: ">= 0",
		Got:  "-1",
	})
	if snap.getCalls != 0 {
		t.Fatalf("getCalls = %d, want 0 for invalid input", snap.getCalls)
	}
}

func TestJudgePassesWithoutWrites(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_SCENE:    "S001-KT",
		PATH_ITEMS:    []any{itemRow("食材", "S001-KT", 2)},
		PATH_MONEY:    100,
		PATH_AFFINITY: 80,
	}}
	flow := spec.FlowSpec{Scene: "S001-KT", Requires: []string{"食材"}, Cost: 10}
	reqs := append(Requirements(flow), Requirement{Kind: KIND_AFFINITY, Threshold: 60})
	before := snap.fingerprint(t)

	first := Judge(flow, snap)
	second := Judge(flow, snap)
	if !first.OK || len(first.Reasons) != 0 {
		t.Fatalf("first verdict = %+v, want pass", first)
	}
	if first.Reasons == nil {
		t.Fatal("pass verdict Reasons = nil, want empty non-nil slice")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("replay mismatch:\n first  = %+v\n second = %+v", first, second)
	}

	withAffinityFirst := JudgeRequirements(reqs, snap)
	withAffinitySecond := JudgeRequirements(reqs, snap)
	if !withAffinityFirst.OK {
		t.Fatalf("affinity verdict = %+v, want pass", withAffinityFirst)
	}
	if !reflect.DeepEqual(withAffinityFirst, withAffinitySecond) {
		t.Fatalf("replay mismatch:\n first  = %+v\n second = %+v", withAffinityFirst, withAffinitySecond)
	}
	assertUntouched(t, snap, before)
}

func TestSameNameItemsUseTemplateScene(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_SCENE: "S001-KT",
		PATH_ITEMS: []any{
			itemRow("食材", "S008", 5),
			itemRow("食材", "S001-KT", 1),
		},
	}}
	flow := spec.FlowSpec{Scene: "S001-KT", Requires: []string{"食材"}}
	if verdict := Judge(flow, snap); !verdict.OK {
		t.Fatalf("verdict = %+v, want pass for the template-scene item", verdict)
	}
	twoInTemplateScene := JudgeRequirements([]Requirement{{
		Kind: KIND_ITEM, Item: "食材", Scene: "S001-KT", Quantity: 2,
	}}, snap)
	assertSingleReason(t, twoInTemplateScene, Reason{
		Code: REASON_MISSING_ITEM,
		Path: PATH_ITEMS + ".食材",
		Want: "2",
		Got:  "1",
	})
}

func TestEmptySceneIsUnrestricted(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_ITEMS: []any{itemRow("食材", "S008", 1)},
	}}
	verdict := Judge(spec.FlowSpec{Requires: []string{"食材"}}, snap)
	if !verdict.OK {
		t.Fatalf("verdict = %+v, want pass when scene is unconstrained", verdict)
	}
}

func TestItemRequirementDefaultsQuantity(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_ITEMS: []any{itemRow("食材", "", 1)},
	}}
	if verdict := JudgeRequirements([]Requirement{{
		Kind: KIND_ITEM, Item: "食材",
	}}, snap); !verdict.OK {
		t.Fatalf("verdict = %+v, want pass with default quantity 1", verdict)
	}
	needsTwo := JudgeRequirements([]Requirement{{
		Kind: KIND_ITEM, Item: "食材", Quantity: 2,
	}}, snap)
	assertSingleReason(t, needsTwo, Reason{
		Code: REASON_MISSING_ITEM,
		Path: PATH_ITEMS + ".食材",
		Want: "2",
		Got:  "1",
	})
}

func TestItemViewMapShape(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_SCENE: "S001-KT",
		PATH_ITEMS: map[string]any{
			"食材": map[string]any{
				KEY_ITEM_SCENE:    "S001-KT",
				KEY_ITEM_QUANTITY: 2,
			},
		},
	}}
	verdict := Judge(spec.FlowSpec{Scene: "S001-KT", Requires: []string{"食材"}}, snap)
	if !verdict.OK {
		t.Fatalf("verdict = %+v, want pass for a name-keyed item view", verdict)
	}
}

func TestItemViewDecodeErrorIsOneEvalError(t *testing.T) {
	snap := &recordingSnapshot{data: map[string]any{
		PATH_SCENE: "S001-KT",
		PATH_ITEMS: []any{"not-an-item-record"},
	}}
	verdict := Judge(spec.FlowSpec{Scene: "S001-KT", Requires: []string{"食材", "食物"}}, snap)
	if verdict.OK || len(verdict.Reasons) != 1 {
		t.Fatalf("verdict = %+v, want one eval_error for the shared item view", verdict)
	}
	if reason := verdict.Reasons[0]; reason.Code != REASON_EVAL_ERROR || reason.Path != PATH_ITEMS {
		t.Fatalf("reason = %+v, want eval_error at %s", reason, PATH_ITEMS)
	}
}

func TestRequirementsDerivation(t *testing.T) {
	got := Requirements(spec.FlowSpec{
		Scene:    "S001-KT",
		Requires: []string{"食材", "食物"},
		Cost:     3,
	})
	want := []Requirement{
		{Kind: KIND_SCENE, Scene: "S001-KT"},
		{Kind: KIND_ITEM, Item: "食材", Scene: "S001-KT", Quantity: 1},
		{Kind: KIND_ITEM, Item: "食物", Scene: "S001-KT", Quantity: 1},
		{Kind: KIND_MONEY, Threshold: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requirements = %+v, want %+v", got, want)
	}
	if empty := Requirements(spec.FlowSpec{}); len(empty) != 0 {
		t.Fatalf("empty flow requirements = %+v, want none", empty)
	}
}
