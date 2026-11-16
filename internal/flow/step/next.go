// next.go advances one flow step and accumulates its minutes.
package step

import (
	"fmt"
	"math"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Next consumes the current step with minutes spent and advances the K3 state.
// Only pending or running states may advance: an interrupted execution must be
// resumed first, and done is terminal. StepIndex moves forward by at most one
// and MinutesSpent grows by exactly minutes, never backwards; an addition that
// would overflow is refused instead of wrapping into a regression.
func Next(flow *spec.FlowSpec, state State, minutes int) (State, error) {
	if err := validateFlow(flow); err != nil {
		return State{}, err
	}
	if err := validateStepIndex(state, len(flow.Steps)); err != nil {
		return State{}, err
	}
	if minutes < 0 {
		return State{}, fmt.Errorf("%w: %d", ErrNegativeMinutes, minutes)
	}
	if minutes > math.MaxInt-state.MinutesSpent {
		return State{}, fmt.Errorf("%w: %d + %d", ErrMinutesOverflow, state.MinutesSpent, minutes)
	}
	switch state.Status {
	case StatusPending, StatusRunning:
	default:
		return State{}, fmt.Errorf("%w: next from %s", ErrInvalidTransition, state.Status)
	}

	next, err := transition(state, StatusRunning)
	if err != nil {
		return State{}, err
	}
	next.MinutesSpent += minutes
	if next.StepIndex == len(flow.Steps)-1 {
		return transition(next, StatusDone)
	}
	next.StepIndex++
	return next, nil
}
