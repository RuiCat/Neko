// errors.go declares the sentinel errors of the K3 step state machine.
package step

import "errors"

var (
	// ErrStaleExecution means a caller tried to re-enter an execution whose
	// execution_id does not match the persisted K3 state.
	ErrStaleExecution = errors.New("step: stale execution")
	// ErrStepOutOfRange means StepIndex is outside the flow's steps slice, so
	// no step can be started or advanced.
	ErrStepOutOfRange = errors.New("step: step index out of range")
	// ErrInvalidTransition means the K3 status transition table forbids the
	// requested status change (for example Next after done).
	ErrInvalidTransition = errors.New("step: invalid status transition")
	// ErrNegativeMinutes means a Next call tried to subtract minutes; spent
	// minutes must never regress.
	ErrNegativeMinutes = errors.New("step: negative minutes")
	// ErrMinutesOverflow means adding a step's minutes would wrap MinutesSpent,
	// which would silently make the cumulative spend regress.
	ErrMinutesOverflow = errors.New("step: minutes overflow")
	// ErrInvalidFlow means the flow is nil or has no id to bind the state to.
	ErrInvalidFlow = errors.New("step: invalid flow spec")
	// ErrEmptyExecutionID means Start was called without an idempotency key.
	ErrEmptyExecutionID = errors.New("step: empty execution id")
)
