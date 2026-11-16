// state.go declares the frozen K3 step.State wire shape and its four statuses.
// The package tracks flow-level step progress only; the action kernel (行动)
// remains the single action state machine and no field here shadows it (C6).
package step

import "fmt"

// STATE_VERSION is the K3 state contract version. K3 freezes the State field
// list, so the version is an API constant rather than an extra State field.
const STATE_VERSION = 1

// Status is one step-execution status. The four values below are frozen by K3;
// no other status may be introduced without a contract change.
type Status string

const (
	// StatusPending means the execution exists but no step has been consumed.
	StatusPending Status = "pending"
	// StatusRunning means a step is in progress and Next may advance it.
	StatusRunning Status = "running"
	// StatusInterrupted means progress is parked until Resume.
	StatusInterrupted Status = "interrupted"
	// StatusDone is terminal: every step was consumed exactly once.
	StatusDone Status = "done"
)

// State is the K3 record of one flow execution. StepIndex is the 0-based index
// of the current step; it advances after each successful Next and stays on the
// last step once Status is done. MinutesSpent is cumulative and only grows.
// StartedTick is the world tick that Start recorded verbatim.
type State struct {
	ExecutionID  string `json:"executionID"`
	FlowID       string `json:"flowID"`
	StepIndex    int    `json:"stepIndex"`
	Status       Status `json:"status"`
	MinutesSpent int    `json:"minutesSpent"`
	StartedTick  int    `json:"startedTick"`
}

// CheckExecution reports ErrStaleExecution when the state belongs to another
// execution. K3 requires a mismatched execution_id to be refused on re-entry;
// because transitions are pure and carry their identity in State, callers that
// re-enter a persisted execution use this guard before a transition.
func (s State) CheckExecution(executionID string) error {
	if s.ExecutionID != executionID {
		return fmt.Errorf("%w: state is %q, caller is %q", ErrStaleExecution, s.ExecutionID, executionID)
	}
	return nil
}
