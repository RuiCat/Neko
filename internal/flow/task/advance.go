// advance.go is the K4 consumer: one Completion event advances its task template
// from 未激活 to 完成, writes the two task state rows exactly once and pays the
// template reward exactly once.
package task

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Advance consumes one K4 completion and advances the task named by its flow ID.
// An unknown flow ID, a non-完成 result, a missing entity or a stage outside the
// contract is refused with a sentinel error and writes nothing. A completion whose
// (flowID, entity) pair is already in the ledger, or whose task stage row already
// reads 完成, returns a Replay transition and writes nothing (C4 single
// settlement). The transition lists the stage path traversed and the reward rows
// applied.
func Advance(completion settle.Completion, templates []spec.TaskTemplateSpec, deps Deps) (Transition, error) {
	if err := checkAdvanceInput(completion, deps); err != nil {
		return Transition{}, err
	}
	template, found := findTemplate(templates, completion.FlowID)
	if !found {
		return Transition{}, fmt.Errorf("%w: %q", ErrUnknownFlow, completion.FlowID)
	}
	if err := checkReward(template.Reward); err != nil {
		return Transition{}, err
	}
	if recorded, replayed := deps.Ledger.Lookup(completion.FlowID, completion.Entity); replayed {
		recorded.Replay = true
		recorded.Rewards = []Reward{}
		return recorded, nil
	}
	from, err := readStage(deps.Store, completion.Entity)
	if err != nil {
		return Transition{}, err
	}
	if from == STAGE_COMPLETED {
		return replayTransition(completion, from), nil
	}
	if err := activate(completion, deps.Store); err != nil {
		return Transition{}, err
	}
	if err := deps.Store.Set(completion.Entity, PATH_TASK_PROGRESS, PROGRESS_COMPLETE); err != nil {
		return Transition{}, fmt.Errorf("task %q: write progress: %w", completion.FlowID, err)
	}
	rewards, err := payRewards(deps.Store, completion.Entity, template.Reward)
	if err != nil {
		return Transition{}, err
	}
	transition := Transition{
		FlowID:    completion.FlowID,
		Entity:    completion.Entity,
		Tick:      completion.Tick,
		FromStage: from,
		ToStage:   STAGE_COMPLETED,
		Stages:    stagePath(from),
		Progress:  PROGRESS_COMPLETE,
		Rewards:   rewards,
	}
	deps.Ledger.Record(transition)
	return transition, nil
}

// checkAdvanceInput refuses a completion that cannot drive a task at all. The
// store and ledger are checked first because every later step needs them.
func checkAdvanceInput(completion settle.Completion, deps Deps) error {
	if deps.Store == nil {
		return ErrMissingStore
	}
	if deps.Ledger == nil {
		return ErrMissingLedger
	}
	if strings.TrimSpace(completion.Entity) == "" {
		return ErrMissingEntity
	}
	if completion.Result != settle.RESULT_COMPLETED {
		return fmt.Errorf("%w: %q", ErrNotCompleted, completion.Result)
	}
	return nil
}

// findTemplate looks the task template up by flow ID: the corpus task libraries
// name each template after the task, so the completion flow ID is the template ID.
func findTemplate(templates []spec.TaskTemplateSpec, flowID string) (spec.TaskTemplateSpec, bool) {
	for _, template := range templates {
		if template.ID == flowID {
			return template, true
		}
	}
	return spec.TaskTemplateSpec{}, false
}

// activate writes the task identity row only when it changes. The rules' tkStart
// rule fires on a non-empty 任务.当前 while the stage reads 未激活, so this write is
// the activation edge of the stage machine.
func activate(completion settle.Completion, store Store) error {
	current, ok := store.Get(completion.Entity, PATH_TASK_CURRENT)
	if ok && current == completion.FlowID {
		return nil
	}
	if err := store.Set(completion.Entity, PATH_TASK_CURRENT, completion.FlowID); err != nil {
		return fmt.Errorf("task %q: write current task: %w", completion.FlowID, err)
	}
	return nil
}

// replayTransition is the store-backed half of single settlement: the stage row
// already reads 完成, so an earlier process paid this completion and this call must
// not pay it again (C4). The ledger covers the same case inside one process.
func replayTransition(completion settle.Completion, stage string) Transition {
	return Transition{
		FlowID:    completion.FlowID,
		Entity:    completion.Entity,
		Tick:      completion.Tick,
		FromStage: stage,
		ToStage:   stage,
		Stages:    []string{stage},
		Progress:  PROGRESS_COMPLETE,
		Rewards:   []Reward{},
		Replay:    true,
	}
}
