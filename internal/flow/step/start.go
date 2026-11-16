// start.go creates the pending K3 state that begins one flow execution.
package step

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Start binds executionID to flow and returns the pending state at step 0,
// recording tick as StartedTick. Repeating Start with the same arguments is
// idempotent: it yields an equal pending state and never advances a step.
// A flow without steps has no valid step index and is refused with
// ErrStepOutOfRange instead of being reported as instantly done.
func Start(flow *spec.FlowSpec, executionID string, tick int) (State, error) {
	if err := validateFlow(flow); err != nil {
		return State{}, err
	}
	if executionID == "" {
		return State{}, fmt.Errorf("%w: flow %q", ErrEmptyExecutionID, flow.ID)
	}
	if len(flow.Steps) == 0 {
		return State{}, fmt.Errorf("%w: flow %q has no steps", ErrStepOutOfRange, flow.ID)
	}
	return State{
		ExecutionID:  executionID,
		FlowID:       flow.ID,
		StepIndex:    0,
		Status:       StatusPending,
		MinutesSpent: 0,
		StartedTick:  tick,
	}, nil
}
