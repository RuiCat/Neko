// errors.go lists the refusal conditions of task advancement. Every refusal is a
// sentinel so a caller can tell an unknown flow from an already advanced
// completion without parsing prose, and so none of them is silently skipped.
package task

import "errors"

var (
	// ErrUnknownFlow means the completion's flow ID has no task template.
	ErrUnknownFlow = errors.New("task: completion flow has no task template")
	// ErrNotCompleted means the K4 result is not the 完成 result.
	ErrNotCompleted = errors.New("task: completion result is not 完成")
	// ErrMissingEntity means the completion carries no entity.
	ErrMissingEntity = errors.New("task: completion has no entity")
	// ErrUnknownStage means the stage row holds a value outside the task contract.
	ErrUnknownStage = errors.New("task: unknown task stage")
	// ErrNegativeReward means a template reward is negative; rewards are additive.
	ErrNegativeReward = errors.New("task: negative task reward")
	// ErrMissingStore means Deps carries no store.
	ErrMissingStore = errors.New("task: store is nil")
	// ErrMissingLedger means Deps carries no ledger.
	ErrMissingLedger = errors.New("task: ledger is nil")
)
