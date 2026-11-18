// stage_test.go asserts the stage machine mapping: 未激活 → 进行中 → 完成, the
// replay edge for an already finished stage, and which rows each edge writes.
package task

import (
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestStageMapping covers every entry stage the contract allows plus the absent
// row, and asserts the traversed path, the activation edge and the completion edge.
func TestStageMapping(t *testing.T) {
	tests := []struct {
		name          string
		stage         any
		current       string
		from          string
		stages        []string
		currentWrites int
		replay        bool
	}{
		{
			name:          "absent stage row",
			stage:         nil,
			from:          STAGE_INACTIVE,
			stages:        []string{STAGE_ACTIVE, STAGE_COMPLETED},
			currentWrites: 1,
		},
		{
			name:          "inactive stage",
			stage:         STAGE_INACTIVE,
			from:          STAGE_INACTIVE,
			stages:        []string{STAGE_ACTIVE, STAGE_COMPLETED},
			currentWrites: 1,
		},
		{
			name:          "already active stage",
			stage:         STAGE_ACTIVE,
			current:       "收集任务",
			from:          STAGE_ACTIVE,
			stages:        []string{STAGE_COMPLETED},
			currentWrites: 0,
		},
		{
			name:          "already completed stage",
			stage:         STAGE_COMPLETED,
			current:       "收集任务",
			from:          STAGE_COMPLETED,
			stages:        []string{STAGE_COMPLETED},
			currentWrites: 0,
			replay:        true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore(map[string]any{PATH_RELATION_AFFINITY: 50})
			if test.stage != nil {
				store.values[storeKey(TEST_ENTITY, PATH_TASK_STAGE)] = test.stage
			}
			if test.current != "" {
				store.values[storeKey(TEST_ENTITY, PATH_TASK_CURRENT)] = test.current
			}
			transition, err := Advance(completionFixture(), []spec.TaskTemplateSpec{rewardTemplate()},
				Deps{Store: store, Ledger: NewMemoryLedger()})
			if err != nil {
				t.Fatalf("Advance: %v", err)
			}
			if transition.FromStage != test.from {
				t.Fatalf("from stage = %q, want %q", transition.FromStage, test.from)
			}
			if transition.ToStage != STAGE_COMPLETED {
				t.Fatalf("to stage = %q, want %q", transition.ToStage, STAGE_COMPLETED)
			}
			if !reflect.DeepEqual(transition.Stages, test.stages) {
				t.Fatalf("stages = %v, want %v", transition.Stages, test.stages)
			}
			if transition.Replay != test.replay {
				t.Fatalf("replay = %v, want %v", transition.Replay, test.replay)
			}
			if got := store.writeCount(PATH_TASK_CURRENT); got != test.currentWrites {
				t.Fatalf("current-task writes = %d, want %d", got, test.currentWrites)
			}
			if test.replay {
				if got := store.totalWrites(); got != 0 {
					t.Fatalf("replay wrote %d time(s)", got)
				}
				return
			}
			if got := store.valueAt(PATH_TASK_PROGRESS); got != PROGRESS_COMPLETE {
				t.Fatalf("progress = %v, want %d", got, PROGRESS_COMPLETE)
			}
			if got := store.writeCount(PATH_GROWTH_EXPERIENCE); got != 1 {
				t.Fatalf("experience writes = %d, want 1", got)
			}
			if got := store.writeCount(PATH_TASK_STAGE); got != 0 {
				t.Fatalf("stage row written %d time(s): it is rule-owned (C4)", got)
			}
		})
	}
}

// TestStageReplaySkipsReward proves the store-backed half of single settlement: a
// completed stage row replays even when the ledger lost its record, so a second
// process cannot pay the reward again.
func TestStageReplaySkipsReward(t *testing.T) {
	store := newFakeStore(map[string]any{
		PATH_TASK_STAGE:        STAGE_COMPLETED,
		PATH_TASK_CURRENT:      "收集任务",
		PATH_GROWTH_EXPERIENCE: 20,
		PATH_RELATION_AFFINITY: 55,
	})
	transition, err := Advance(completionFixture(), []spec.TaskTemplateSpec{rewardTemplate()},
		Deps{Store: store, Ledger: NewMemoryLedger()})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if !transition.Replay {
		t.Fatal("completed stage must replay instead of paying again")
	}
	if got := store.totalWrites(); got != 0 {
		t.Fatalf("replay wrote %d time(s)", got)
	}
	if got := store.intAt(t, PATH_GROWTH_EXPERIENCE); got != 20 {
		t.Fatalf("experience = %d, want 20 unchanged", got)
	}
}
