// advance_test.go asserts the C4 behavioural invariants of Advance: one completion
// pays the reward once, refusals write nothing, and the rule-owned stage row is
// never written by this package.
package task

import (
	"errors"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestTaskSingleSettlement advances the same completion twice and asserts the two
// reward rows grew exactly once each (C4 / V2).
func TestTaskSingleSettlement(t *testing.T) {
	store := newFakeStore(map[string]any{
		PATH_TASK_STAGE:        STAGE_INACTIVE,
		PATH_RELATION_AFFINITY: 50,
	})
	deps := Deps{Store: store, Ledger: NewMemoryLedger()}
	templates := []spec.TaskTemplateSpec{rewardTemplate()}
	completion := completionFixture()

	first, err := Advance(completion, templates, deps)
	if err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if first.Replay {
		t.Fatal("the first Advance must not be a replay")
	}
	second, err := Advance(completion, templates, deps)
	if err != nil {
		t.Fatalf("second Advance: %v", err)
	}
	if !second.Replay {
		t.Fatal("the second Advance must replay the recorded transition")
	}
	if len(second.Rewards) != 0 {
		t.Fatalf("replay applied rewards: %+v", second.Rewards)
	}
	if got := store.intAt(t, PATH_GROWTH_EXPERIENCE); got != 20 {
		t.Fatalf("experience after two advances = %d, want 20", got)
	}
	if got := store.intAt(t, PATH_RELATION_AFFINITY); got != 55 {
		t.Fatalf("affinity after two advances = %d, want 55 (50 base + 5 once)", got)
	}
	if got := store.writeCount(PATH_GROWTH_EXPERIENCE); got != 1 {
		t.Fatalf("experience writes = %d, want 1", got)
	}
	if got := store.writeCount(PATH_RELATION_AFFINITY); got != 1 {
		t.Fatalf("affinity writes = %d, want 1", got)
	}
	if got := store.valueAt(PATH_TASK_PROGRESS); got != PROGRESS_COMPLETE {
		t.Fatalf("progress = %v, want %d", got, PROGRESS_COMPLETE)
	}
	if got := store.valueAt(PATH_TASK_CURRENT); got != completion.FlowID {
		t.Fatalf("current task = %v, want %q", got, completion.FlowID)
	}
	if got := store.writeCount(PATH_TASK_STAGE); got != 0 {
		t.Fatalf("stage row written %d time(s): it is rule-owned (C4)", got)
	}
}

// TestAdvanceRefusals asserts every refusal path returns its sentinel error and
// leaves the store untouched.
func TestAdvanceRefusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*settle.Completion, *[]spec.TaskTemplateSpec, *Deps)
		want   error
	}{
		{
			name: "unknown flow",
			mutate: func(_ *settle.Completion, templates *[]spec.TaskTemplateSpec, _ *Deps) {
				*templates = nil
			},
			want: ErrUnknownFlow,
		},
		{
			name: "result is not 完成",
			mutate: func(completion *settle.Completion, _ *[]spec.TaskTemplateSpec, _ *Deps) {
				completion.Result = "失败"
			},
			want: ErrNotCompleted,
		},
		{
			name: "missing entity",
			mutate: func(completion *settle.Completion, _ *[]spec.TaskTemplateSpec, _ *Deps) {
				completion.Entity = "  "
			},
			want: ErrMissingEntity,
		},
		{
			name: "unmapped stage value",
			mutate: func(_ *settle.Completion, _ *[]spec.TaskTemplateSpec, deps *Deps) {
				deps.Store = newFakeStore(map[string]any{PATH_TASK_STAGE: "已激活"})
			},
			want: ErrUnknownStage,
		},
		{
			name: "negative reward",
			mutate: func(_ *settle.Completion, templates *[]spec.TaskTemplateSpec, _ *Deps) {
				*templates = []spec.TaskTemplateSpec{{ID: "收集任务", Reward: spec.TaskReward{Experience: -1}}}
			},
			want: ErrNegativeReward,
		},
		{
			name: "missing store",
			mutate: func(_ *settle.Completion, _ *[]spec.TaskTemplateSpec, deps *Deps) {
				deps.Store = nil
			},
			want: ErrMissingStore,
		},
		{
			name: "missing ledger",
			mutate: func(_ *settle.Completion, _ *[]spec.TaskTemplateSpec, deps *Deps) {
				deps.Ledger = nil
			},
			want: ErrMissingLedger,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore(map[string]any{PATH_TASK_STAGE: STAGE_INACTIVE})
			templates := []spec.TaskTemplateSpec{rewardTemplate()}
			completion := completionFixture()
			deps := Deps{Store: store, Ledger: NewMemoryLedger()}
			test.mutate(&completion, &templates, &deps)

			transition, err := Advance(completion, templates, deps)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if !isEmptyTransition(transition) {
				t.Fatalf("refusal returned a transition: %+v", transition)
			}
			if got := store.totalWrites(); got != 0 {
				t.Fatalf("refusal wrote %d time(s)", got)
			}
		})
	}
}

// TestAdvanceStoreFailurePropagates asserts a failing store write aborts the
// advance instead of reporting a successful transition.
func TestAdvanceStoreFailurePropagates(t *testing.T) {
	store := newFakeStore(map[string]any{PATH_TASK_STAGE: STAGE_INACTIVE})
	store.failOn = PATH_TASK_PROGRESS
	transition, err := Advance(completionFixture(), []spec.TaskTemplateSpec{rewardTemplate()},
		Deps{Store: store, Ledger: NewMemoryLedger()})
	if err == nil {
		t.Fatalf("expected a store error, got transition %+v", transition)
	}
	if !isEmptyTransition(transition) {
		t.Fatalf("failed advance returned a transition: %+v", transition)
	}
}

// TestAdvanceWriteSetIsClosed asserts the writer role C4 assigns to this package
// is closed at run time: one advance writes only the four declared rows and never
// the rule-owned stage row. It is the behavioural witness behind the textual
// single-writer check in writer_test.go.
func TestAdvanceWriteSetIsClosed(t *testing.T) {
	allowed := map[string]struct{}{
		PATH_TASK_CURRENT:      {},
		PATH_TASK_PROGRESS:     {},
		PATH_GROWTH_EXPERIENCE: {},
		PATH_RELATION_AFFINITY: {},
	}
	store := newFakeStore(map[string]any{PATH_TASK_STAGE: STAGE_INACTIVE})
	if _, err := Advance(completionFixture(), []spec.TaskTemplateSpec{rewardTemplate()},
		Deps{Store: store, Ledger: NewMemoryLedger()}); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if store.totalWrites() == 0 {
		t.Fatal("advance wrote nothing")
	}
	for _, write := range store.writes {
		if _, ok := allowed[write.path]; !ok {
			t.Fatalf("advance wrote undeclared path %q", write.path)
		}
	}
}
