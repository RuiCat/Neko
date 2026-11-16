// next_test.go covers step advancement, bounds, minutes and replay idempotency.
package step

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

func TestNextAdvancesSteps(t *testing.T) {
	flow := newFlowFixture()
	state := mustStart(t, flow)

	state = mustNext(t, flow, state, 10)
	if state.Status != StatusRunning || state.StepIndex != 1 || state.MinutesSpent != 10 {
		t.Fatalf("after first Next = %+v, want running step 1 with 10 minutes", state)
	}

	state = mustNext(t, flow, state, 5)
	if state.Status != StatusRunning || state.StepIndex != 2 || state.MinutesSpent != 15 {
		t.Fatalf("after second Next = %+v, want running step 2 with 15 minutes", state)
	}

	state = mustNext(t, flow, state, 7)
	if state.Status != StatusDone || state.StepIndex != 2 || state.MinutesSpent != 22 {
		t.Fatalf("after final Next = %+v, want done at last step with 22 minutes", state)
	}
}

func TestNextRejectsOutOfRangeStep(t *testing.T) {
	flow := newFlowFixture()
	cases := []struct {
		name  string
		state State
	}{
		{"negative index", State{ExecutionID: "exec-1", FlowID: flow.ID, StepIndex: -1, Status: StatusRunning}},
		{"index at length", State{ExecutionID: "exec-1", FlowID: flow.ID, StepIndex: len(flow.Steps), Status: StatusRunning}},
		{"index far past length", State{ExecutionID: "exec-1", FlowID: flow.ID, StepIndex: 99, Status: StatusPending}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Next(flow, tc.state, 1); !errors.Is(err, ErrStepOutOfRange) {
				t.Fatalf("Next error = %v, want ErrStepOutOfRange", err)
			}
		})
	}
}

func TestNextRejectsInvalidInput(t *testing.T) {
	flow := newFlowFixture()
	state := mustStart(t, flow)

	if _, err := Next(nil, state, 1); !errors.Is(err, ErrInvalidFlow) {
		t.Fatalf("Next(nil flow) = %v, want ErrInvalidFlow", err)
	}
	if _, err := Next(flow, state, -1); !errors.Is(err, ErrNegativeMinutes) {
		t.Fatalf("Next(-1 minute) = %v, want ErrNegativeMinutes", err)
	}
	overflow := State{
		ExecutionID:  state.ExecutionID,
		FlowID:       flow.ID,
		StepIndex:    0,
		Status:       StatusRunning,
		MinutesSpent: math.MaxInt - 1,
	}
	if _, err := Next(flow, overflow, 2); !errors.Is(err, ErrMinutesOverflow) {
		t.Fatalf("Next(overflow) = %v, want ErrMinutesOverflow", err)
	}
}

func TestNextRejectsInterruptedAndDone(t *testing.T) {
	flow := newFlowFixture()
	interrupted := mustInterrupt(t, flow)
	if _, err := Next(flow, interrupted, 1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Next(interrupted) = %v, want ErrInvalidTransition", err)
	}

	done := mustFinish(t, flow)
	if _, err := Next(flow, done, 1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Next(done) = %v, want ErrInvalidTransition", err)
	}
}

// TestNextReplayDoesNotDoubleAdvance replays the same transition from the same
// persisted state and requires the result to be identical, so a retried Next
// cannot advance a step twice.
func TestNextReplayDoesNotDoubleAdvance(t *testing.T) {
	flow := newFlowFixture()
	start := mustStart(t, flow)

	first, err := Next(flow, start, 10)
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	replay, err := Next(flow, start, 10)
	if err != nil {
		t.Fatalf("replayed Next: %v", err)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Fatalf("replayed Next differs: %+v != %+v", first, replay)
	}
	if start.Status != StatusPending || start.StepIndex != 0 || start.MinutesSpent != 0 {
		t.Fatalf("input state mutated: %+v", start)
	}
	if first.StepIndex != 1 || first.MinutesSpent != 10 {
		t.Fatalf("advanced state = %+v, want step 1 with 10 minutes", first)
	}

	third := mustNext(t, flow, first, 10)
	if third.StepIndex != 2 {
		t.Fatalf("next distinct step index = %d, want 2", third.StepIndex)
	}
}

func TestMinutesSpentNeverRegresses(t *testing.T) {
	flow := newFlowFixture()
	state := mustStart(t, flow)
	previous := state.MinutesSpent
	check := func(current State) {
		t.Helper()
		if current.MinutesSpent < previous {
			t.Fatalf("MinutesSpent regressed: %d -> %d", previous, current.MinutesSpent)
		}
		previous = current.MinutesSpent
	}

	state = mustNext(t, flow, state, 10)
	check(state)

	interrupted, err := Interrupt(state)
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	check(interrupted)

	resumed, err := Resume(interrupted)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	check(resumed)

	state = mustNext(t, flow, resumed, 3)
	check(state)
	state = mustNext(t, flow, state, 4)
	check(state)
}

func TestNextSingleStepFlowCompletes(t *testing.T) {
	flow := &spec.FlowSpec{ID: "一步", Steps: []string{"一步"}, Cost: 1}
	state := mustNext(t, flow, mustStart(t, flow), 5)
	if state.Status != StatusDone || state.StepIndex != 0 || state.MinutesSpent != 5 {
		t.Fatalf("single-step Next = %+v, want done at step 0 with 5 minutes", state)
	}
}
