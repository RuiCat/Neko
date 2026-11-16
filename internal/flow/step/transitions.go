// transitions.go holds the explicit K3 status transition table. Operations
// still enforce their own from-status; the table is the single source of legal
// status pairs so an illegal migration always returns ErrInvalidTransition.
package step

import "fmt"

// transitions lists every legal status change. The running self-transition is
// present so Next can advance a step without changing status between steps.
var transitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusRunning: true,
	},
	StatusRunning: {
		StatusRunning:     true,
		StatusInterrupted: true,
		StatusDone:        true,
	},
	StatusInterrupted: {
		StatusRunning: true,
	},
}

// isTransitionAllowed reports whether the frozen table permits from -> to.
func isTransitionAllowed(from, to Status) bool {
	return transitions[from][to]
}

// transition applies to to the state or returns ErrInvalidTransition. It never
// touches StepIndex, MinutesSpent or StartedTick; those belong to the callers.
func transition(state State, to Status) (State, error) {
	if !isTransitionAllowed(state.Status, to) {
		return State{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, state.Status, to)
	}
	state.Status = to
	return state, nil
}
