// pause_test.go covers the Interrupt and Resume transitions and their guards.
package step

import (
	"errors"
	"testing"
)

func TestInterruptResumePreservesProgress(t *testing.T) {
	flow := newFlowFixture()
	running := mustNext(t, flow, mustStart(t, flow), 10)

	interrupted, err := Interrupt(running)
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if interrupted.Status != StatusInterrupted {
		t.Fatalf("Status = %s, want %s", interrupted.Status, StatusInterrupted)
	}
	if interrupted.StepIndex != running.StepIndex || interrupted.MinutesSpent != running.MinutesSpent {
		t.Fatalf("Interrupt changed progress: %+v -> %+v", running, interrupted)
	}
	if interrupted.StartedTick != running.StartedTick {
		t.Fatalf("Interrupt changed StartedTick: %d -> %d", running.StartedTick, interrupted.StartedTick)
	}

	resumed, err := Resume(interrupted)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Status != StatusRunning {
		t.Fatalf("Status = %s, want %s", resumed.Status, StatusRunning)
	}
	if resumed.StepIndex != interrupted.StepIndex || resumed.MinutesSpent != interrupted.MinutesSpent {
		t.Fatalf("Resume changed progress: %+v -> %+v", interrupted, resumed)
	}
	if resumed.StartedTick != interrupted.StartedTick {
		t.Fatalf("Resume changed StartedTick: %d -> %d", interrupted.StartedTick, resumed.StartedTick)
	}
}

func TestInterruptRejectsNonRunning(t *testing.T) {
	flow := newFlowFixture()
	pending := mustStart(t, flow)
	interrupted := mustInterrupt(t, flow)
	done := mustFinish(t, flow)
	unknown := State{Status: Status("bogus")}

	for _, state := range []State{pending, interrupted, done, unknown} {
		if _, err := Interrupt(state); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("Interrupt(%s) = %v, want ErrInvalidTransition", state.Status, err)
		}
	}
}

func TestResumeRejectsNonInterrupted(t *testing.T) {
	flow := newFlowFixture()
	pending := mustStart(t, flow)
	running := mustNext(t, flow, pending, 10)
	done := mustFinish(t, flow)
	unknown := State{Status: Status("bogus")}

	for _, state := range []State{pending, running, done, unknown} {
		if _, err := Resume(state); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("Resume(%s) = %v, want ErrInvalidTransition", state.Status, err)
		}
	}
}
