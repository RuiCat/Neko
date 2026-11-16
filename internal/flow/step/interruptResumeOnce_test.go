// interruptResumeOnce_test.go is the C6 acceptance test: interrupting a flow
// after tick 1 and resuming it must charge the template exactly once and keep
// the minutes already spent.
package step

import (
	"errors"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// fakeLedger stands in for the m4 settlement layer so the test can observe what
// would be charged. It is test-local: this package never writes currency.
type fakeLedger struct {
	minutes     int
	currency    int
	completions int
}

// chargeMinutes records one step duration once per successful Next.
func (l *fakeLedger) chargeMinutes(minutes int) {
	l.minutes += minutes
}

// settleAtDone charges the template cost only on the edge into done.
func (l *fakeLedger) settleAtDone(cost int, from, to State) {
	if from.Status != StatusDone && to.Status == StatusDone {
		l.currency += cost
		l.completions++
	}
}

// runAndInterrupt drives flow to done and interrupts after the interruptAfter-th
// step; a negative interruptAfter runs without interruption. stepMinutes is the
// duration charged by each successful Next.
func runAndInterrupt(t *testing.T, flow *spec.FlowSpec, stepMinutes, interruptAfter int) (State, fakeLedger) {
	t.Helper()
	state, err := Start(flow, "exec-tick-1", 1)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ledger := fakeLedger{}
	previousMinutes := state.MinutesSpent
	for steps := 0; state.Status != StatusDone; steps++ {
		if steps > len(flow.Steps)+1 {
			t.Fatalf("execution did not finish after %d steps", steps)
		}
		if state.Status == StatusInterrupted {
			state, err = Resume(state)
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
		}
		from := state
		next, err := Next(flow, state, stepMinutes)
		if err != nil {
			t.Fatalf("Next step %d: %v", steps, err)
		}
		ledger.chargeMinutes(stepMinutes)
		ledger.settleAtDone(flow.Cost, from, next)
		if next.MinutesSpent < previousMinutes {
			t.Fatalf("MinutesSpent regressed: %d -> %d", previousMinutes, next.MinutesSpent)
		}
		previousMinutes = next.MinutesSpent
		state = next
		if steps+1 == interruptAfter && state.Status != StatusDone {
			state, err = Interrupt(state)
			if err != nil {
				t.Fatalf("Interrupt after step %d: %v", steps+1, err)
			}
		}
	}
	return state, ledger
}

// TestInterruptResumeOnce compares an uninterrupted run with a run interrupted
// after tick 1: net minutes and net currency must equal the uninterrupted
// template spend, so interruption never double-charges.
func TestInterruptResumeOnce(t *testing.T) {
	flow := newFlowFixture()
	baseline, baselineLedger := runAndInterrupt(t, flow, 10, -1)
	interrupted, interruptedLedger := runAndInterrupt(t, flow, 10, 1)

	if baseline.Status != StatusDone || interrupted.Status != StatusDone {
		t.Fatalf("statuses: baseline=%s interrupted=%s, want done", baseline.Status, interrupted.Status)
	}
	if baselineLedger.currency != flow.Cost {
		t.Fatalf("baseline net cost = %d, want %d", baselineLedger.currency, flow.Cost)
	}
	if interruptedLedger.currency != flow.Cost {
		t.Fatalf("interrupted net cost = %d, want one template cost %d", interruptedLedger.currency, flow.Cost)
	}
	if interruptedLedger.completions != 1 {
		t.Fatalf("done edges = %d, want exactly 1", interruptedLedger.completions)
	}
	if interruptedLedger.minutes != baselineLedger.minutes {
		t.Fatalf("net minutes = %d, want %d (one template spend)", interruptedLedger.minutes, baselineLedger.minutes)
	}
	if interrupted.MinutesSpent != baseline.MinutesSpent {
		t.Fatalf("MinutesSpent = %d, want %d", interrupted.MinutesSpent, baseline.MinutesSpent)
	}
	if interrupted.StartedTick != 1 {
		t.Fatalf("StartedTick = %d, want 1", interrupted.StartedTick)
	}

	// done is terminal: a retried Next cannot create a second done edge/cost.
	if _, err := Next(flow, interrupted, 10); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Next after done error = %v, want ErrInvalidTransition", err)
	}
	if interruptedLedger.currency != flow.Cost {
		t.Fatalf("net cost after retry = %d, want %d", interruptedLedger.currency, flow.Cost)
	}
}
