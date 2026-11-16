// pause.go parks and resumes a running execution without losing progress.
package step

import "fmt"

// Interrupt moves a running execution to interrupted and keeps StepIndex,
// MinutesSpent and StartedTick untouched, so resuming cannot re-spend minutes.
// Pending, interrupted and done states are refused with ErrInvalidTransition.
func Interrupt(state State) (State, error) {
	if state.Status != StatusRunning {
		return State{}, fmt.Errorf("%w: interrupt from %s", ErrInvalidTransition, state.Status)
	}
	return transition(state, StatusInterrupted)
}

// Resume moves an interrupted execution back to running while preserving
// StepIndex, MinutesSpent and StartedTick. Other states are refused with
// ErrInvalidTransition.
func Resume(state State) (State, error) {
	if state.Status != StatusInterrupted {
		return State{}, fmt.Errorf("%w: resume from %s", ErrInvalidTransition, state.Status)
	}
	return transition(state, StatusRunning)
}
